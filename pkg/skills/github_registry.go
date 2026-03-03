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

// GitHubRegistry discovers skills from a GitHub repository's workflow artifacts
// or from a GitHub Gist.
type GitHubRegistry struct {
	config     GitHubRegistryConfig
	httpClient *http.Client
	owner      string
	repo       string
	branch     string
	workflow   string
	gistID    string
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
		gistID:    config.GistID,
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
	index, err := r.getSkillIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill index: %w", err)
	}

	var results []SearchResult
	queryLower := strings.ToLower(query)
	for _, skill := range index.Skills {
		if query == "" || strings.Contains(strings.ToLower(skill.Slug), queryLower) {
			results = append(results, SearchResult{
				Score:        1.0,
				Slug:         skill.Slug,
				DisplayName:  skill.Slug,
				Summary:      skill.Category,
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
				DisplayName:   skill.Slug,
				Summary:       skill.Category,
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

	// Use GitHub Contents API to list and download all files in the skill folder
	if err := r.downloadFolder(ctx, skillDef.Path, targetDir); err != nil {
		return nil, fmt.Errorf("failed to download skill: %w", err)
	}

	return &InstallResult{
		Summary: fmt.Sprintf("Installed skill from %s", skillDef.Path),
	}, nil
}

// downloadFolder downloads all files from a GitHub folder using Contents API.
func (r *GitHubRegistry) downloadFolder(ctx context.Context, path, targetDir string) error {
	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	contentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		r.owner, r.repo, path, r.branch)

	req, err := http.NewRequestWithContext(ctx, "GET", contentsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	if r.config.GHToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.GHToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to list folder contents: %s", resp.Status)
	}

	// GitHub API can return either an array (multiple items) or single object (one item)
	// Try decoding as array first, then as single object if that fails
	var files []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"download_url"`
		Type        string `json:"type"`
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&files); err != nil {
		// Try single object
		resp.Body.Close()
		resp, err = r.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var single struct {
			Name        string `json:"name"`
			DownloadURL string `json:"download_url"`
			Type        string `json:"type"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&single); err != nil {
			return err
		}
		if single.Name != "" {
			files = []struct {
				Name        string `json:"name"`
				DownloadURL string `json:"download_url"`
				Type        string `json:"type"`
			}{single}
		}
	}

	for _, file := range files {
		if file.Type == "dir" {
			subDir := filepath.Join(targetDir, file.Name)
			if err := os.MkdirAll(subDir, 0755); err != nil {
				return err
			}
			if err := r.downloadFolder(ctx, path+"/"+file.Name, subDir); err != nil {
				return err
			}
			continue
		}

		if file.DownloadURL == "" {
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "GET", file.DownloadURL, nil)
		if err != nil {
			return err
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download %s: %s", file.Name, resp.Status)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		outPath := filepath.Join(targetDir, file.Name)
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

// getSkillIndex fetches the skill index from GitHub Gist or workflow artifacts.
func (r *GitHubRegistry) getSkillIndex(ctx context.Context) (*SkillIndex, error) {
	// Fetch skills-index.json from the wiki (no auth needed for public repos)
	// Wiki is accessed via /wiki/ path: https://raw.githubusercontent.com/wiki/owner/repo/filename
	url := fmt.Sprintf("https://raw.githubusercontent.com/wiki/%s/%s/skills-index.json", r.owner, r.repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch skills index: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var index SkillIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// getGistIDFromRepo fetches the gist ID file from the repository.
func (r *GitHubRegistry) getGistIDFromRepo(ctx context.Context, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", r.owner, r.repo, r.branch, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil // No gist ID file yet
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch gist ID file: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getSkillIndexFromArtifact fetches from workflow artifacts (fallback).
func (r *GitHubRegistry) getSkillIndexFromArtifact(ctx context.Context) (*SkillIndex, error) {
	// Use workflow name as artifact name (strip .yml if present)
	artifactName := strings.TrimSuffix(r.workflow, ".yml")

	// Find the latest workflow run
	// Note: GitHub API expects workflow file name (with .yml) not the display name
	workflowFile := r.workflow
	if !strings.HasSuffix(workflowFile, ".yml") && !strings.HasSuffix(workflowFile, ".yaml") {
		workflowFile = workflowFile + ".yml"
	}
	runsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/runs?branch=%s&per_page=1",
		r.owner, r.repo, workflowFile, r.branch)

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
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			ArchiveDownloadURL string `json:"archive_download_url"`
		} `json:"artifacts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&artifactsResp); err != nil {
		return nil, err
	}

	// Find artifact matching workflow name
	var artifact struct {
		ID                 int64  `json:"id"`
		ArchiveDownloadURL string `json:"archive_download_url"`
	}
	found := false
	for _, a := range artifactsResp.Artifacts {
		if a.Name == artifactName {
			artifact = struct {
				ID                 int64  `json:"id"`
				ArchiveDownloadURL string `json:"archive_download_url"`
			}{ID: a.ID, ArchiveDownloadURL: a.ArchiveDownloadURL}
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("artifact not found: %s", artifactName)
	}

	// Download and parse the artifact (still using zip for artifact itself)
	return r.downloadAndParseArtifact(ctx, artifact.ArchiveDownloadURL)
}

// downloadAndParseArtifact downloads and extracts index.json from artifact.
func (r *GitHubRegistry) downloadAndParseArtifact(ctx context.Context, artifactURL string) (*SkillIndex, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", artifactURL, nil)
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
		return nil, fmt.Errorf("failed to download artifact: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract and parse index.json from zip artifact
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact zip: %w", err)
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

// getSkillIndexFromGist fetches the skill index from a public GitHub Gist.
func (r *GitHubRegistry) getSkillIndexFromGist(ctx context.Context, gistID string) (*SkillIndex, error) {
	// Fetch gist JSON (public, no auth needed)
	gistURL := fmt.Sprintf("https://gist.github.com/%s/raw/skills-index.json", gistID)

	req, err := http.NewRequestWithContext(ctx, "GET", gistURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch gist: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var index SkillIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// SkillIndex represents the index published by a GitHub workflow.
type SkillIndex struct {
	Skills []SkillDefinition `json:"skills"`
}

// SkillDefinition defines a skill in the index.
type SkillDefinition struct {
	Slug     string `json:"slug"`     // e.g., "github"
	Category string `json:"category"` // e.g., "picoclaw/skills"
	Path     string `json:"path"`     // e.g., "picoclaw/skills/github"
}
