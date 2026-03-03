package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GitHubRegistry discovers skills from a GitHub repository's workflow artifacts.
// It uses a workflow that publishes a skill index as an artifact.
type GitHubRegistry struct {
	config     GitHubRegistryConfig
	httpClient *http.Client
	owner      string
	repo       string
	branch     string
	workflow   string
}

// NewGitHubRegistry creates a new GitHub registry.
func NewGitHubRegistry(config GitHubRegistryConfig) *GitHubRegistry {
	var tc *http.Client

	if config.GHToken != "" {
		tc = &http.Client{}
	} else {
		tc = http.DefaultClient
	}

	owner, repo, branch, workflow := parseGitHubConfig(config)

	return &GitHubRegistry{
		config:     config,
		httpClient: tc,
		owner:      owner,
		repo:       repo,
		branch:     branch,
		workflow:   workflow,
	}
}

// parseGitHubConfig extracts owner, repo, branch, and workflow from config.
func parseGitHubConfig(config GitHubRegistryConfig) (owner, repo, branch, workflow string) {
	registry := config.Registry
	branch = config.Branch
	if branch == "" {
		branch = "main"
	}
	workflow = config.Workflow
	if workflow == "" {
		workflow = "skills-index.yml"
	}

	// Handle full URL like https://github.company.com/owner/skills
	if strings.HasPrefix(registry, "http") {
		parts := strings.Split(registry, "/")
		if len(parts) >= 4 {
			owner = parts[len(parts)-2]
			repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
		}
	} else {
		// Handle owner/repo shorthand
		parts := strings.Split(registry, "/")
		if len(parts) >= 2 {
			owner = parts[0]
			repo = parts[1]
		}
	}
	return
}

// Name returns the registry name.
func (r *GitHubRegistry) Name() string {
	return "github:" + r.owner + "/" + r.repo
}

// Search searches for skills in the GitHub repository.
func (r *GitHubRegistry) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// First, get the skill index from workflow artifacts
	index, err := r.getSkillIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill index: %w", err)
	}

	// Filter by query
	var results []SearchResult
	queryLower := strings.ToLower(query)
	for _, skill := range index.Skills {
		if query == "" ||
			strings.Contains(strings.ToLower(skill.Name), queryLower) ||
			strings.Contains(strings.ToLower(skill.Summary), queryLower) {
			results = append(results, SearchResult{
				Score:        1.0,
				Slug:         skill.Slug,
				DisplayName:  skill.Name,
				Summary:      skill.Summary,
				Version:      skill.Version,
				RegistryName: r.Name(),
			})
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// GetSkillMeta retrieves metadata for a specific skill.
func (r *GitHubRegistry) GetSkillMeta(ctx context.Context, slug string) (*SkillMeta, error) {
	index, err := r.getSkillIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill index: %w", err)
	}

	for _, skill := range index.Skills {
		if skill.Slug == slug {
			return &SkillMeta{
				Slug:          skill.Slug,
				DisplayName:   skill.Name,
				Summary:       skill.Summary,
				LatestVersion: skill.Version,
				RegistryName:  r.Name(),
			}, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", slug)
}

// DownloadAndInstall downloads and installs a skill from GitHub.
func (r *GitHubRegistry) DownloadAndInstall(ctx context.Context, slug, version, targetDir string) (*InstallResult, error) {
	index, err := r.getSkillIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill index: %w", err)
	}

	// Find the skill
	var skillDef SkillDefinition
	var found bool
	for _, skill := range index.Skills {
		if skill.Slug == slug {
			skillDef = skill
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("skill not found: %s", slug)
	}

	// Download from raw URL
	downloadURL := skillDef.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s.zip",
			r.owner, r.repo, r.branch, slug)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}
	if r.config.GHToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.GHToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	// Read body into bytes for zip.NewReader
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract to target directory
	if err := extractZip(bytes.NewReader(data), targetDir); err != nil {
		return nil, fmt.Errorf("failed to extract skill: %w", err)
	}

	return &InstallResult{
		Version: skillDef.Version,
		Summary: skillDef.Summary,
	}, nil
}

// getSkillIndex fetches the skill index from GitHub workflow artifacts.
func (r *GitHubRegistry) getSkillIndex(ctx context.Context) (*SkillIndex, error) {
	// Find the latest workflow run
	runsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/runs?branch=%s&per_page=1",
		r.owner, r.repo, r.workflow, r.branch)

	req, err := http.NewRequestWithContext(ctx, "GET", runsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	if r.config.GHToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.GHToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list workflow runs: %s", resp.Status)
	}

	var runsResp struct {
		WorkflowRuns []struct {
			ID int64 `json:"id"`
		} `json:"workflow_runs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&runsResp); err != nil {
		return nil, err
	}

	if len(runsResp.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no workflow runs found for %s", r.workflow)
	}

	runID := runsResp.WorkflowRuns[0].ID

	// Get artifacts for this run
	artifactsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/artifacts",
		r.owner, r.repo, runID)

	req, err = http.NewRequestWithContext(ctx, "GET", artifactsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	if r.config.GHToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.GHToken)
	}

	resp, err = r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list artifacts: %s", resp.Status)
	}

	var artifactsResp struct {
		Artifacts []struct {
			ID                int64  `json:"id"`
			ArchiveDownloadURL string `json:"archive_download_url"`
		} `json:"artifacts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&artifactsResp); err != nil {
		return nil, err
	}

	if len(artifactsResp.Artifacts) == 0 {
		return nil, fmt.Errorf("no artifacts found")
	}

	// Download the first artifact (skills-index)
	artifact := artifactsResp.Artifacts[0]

	// Fetch the zip
	req, err = http.NewRequestWithContext(ctx, "GET", artifact.ArchiveDownloadURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err = r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact: %s", resp.Status)
	}

	// Read body into bytes for zip.NewReader
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract and parse index.json
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return nil, err
	}

	for _, f := range zr.File {
		if f.Name == "index.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			var index SkillIndex
			if err := json.Unmarshal(data, &index); err != nil {
				return nil, err
			}
			return &index, nil
		}
	}

	return nil, fmt.Errorf("index.json not found in artifact")
}

// SkillIndex represents the index published by a GitHub workflow.
type SkillIndex struct {
	Skills []SkillDefinition `json:"skills"`
}

// SkillDefinition defines a skill in the index.
type SkillDefinition struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
}

// extractZip extracts a zip reader to a target directory.
func extractZip(reader *bytes.Reader, targetDir string) error {
	zr, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		return err
	}

	for _, f := range zr.File {
		path := filepath.Join(targetDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return err
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}

	return nil
}
