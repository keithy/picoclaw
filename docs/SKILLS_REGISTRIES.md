# Skills Registries

PicoClaw supports installing skills from multiple registries. This guide covers how to use and create registries.

## Using Registries

### List Available Skills

```bash
picoclaw skills search <query>
```

### Install a Skill

```bash
# Install from a specific registry
picoclaw skills install --registry github self-config
picoclaw skills install --registry clawhub github

# Install directly from GitHub
picoclaw skills install owner/repo/skill-name
```

### List Installed Skills

```bash
picoclaw skills list
```

## Supported Registries

### ClawHub

The default registry at [clawhub.ai](https://clawhub.ai). Enable in config:

```json
{
  "tools": {
    "skills": {
      "registries": {
        "clawhub": {
          "enabled": true
        }
      }
    }
  }
}
```

### GitHub

Install skills from any public GitHub repository that publishes a skills index.

**Configuration:**

```json
{
  "tools": {
    "skills": {
      "registries": {
        "github": {
          "enabled": true,
          "registry": "owner/repo",
          "branch": "main",
          "workflow": "skills-index"
        }
      }
    }
  }
}
```

## Creating Your Own Registry

To create a GitHub-based registry:

### 1. Create a GitHub Repository

Create a public repository to host your skills.

### 2. Add Skills

Add skills in the `skills/` directory. Each skill needs a `SKILL.md` file:

```
skills/
├── github/
│   └── SKILL.md
├── weather/
│   └── SKILL.md
└── calculator/
    └── SKILL.md
```

### 3. Create the Index Workflow

Add `.github/workflows/skills-index.yml`:

```yaml
name: Skills Index

on:
  push:
    branches: [main]
    paths:
      - 'skills/**/SKILL.md'
  schedule:
    - cron: '0 0 1 * *'  # Monthly
  workflow_dispatch:

permissions:
  contents: write

jobs:
  index:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate Skills Index
        run: |
          # Find all SKILL.md files and generate index
          index_json=$(find skills -name 'SKILL.md' -type f | while read file; do
            slug=$(basename "$(dirname "$file")")
            category=$(basename "$(dirname "$(dirname "$file")")")
            name=$(sed -n 's/^# *//p' "$file" | head -1)
            echo "{\"slug\":\"$slug\",\"name\":\"$name\",\"category\":\"$category\",\"path\":\"$file\"}"
          done | jq -s '.')

          echo "{\"version\":1,\"skills\":$index_json}" > skills-index.json

      - name: Commit Index to Repo
        uses: stefanzweifel/git-auto-commit-action@v5
        with:
          commit_message: "chore: update skills index"
          file_pattern: "skills-index.json"
```

### 4. Enable in PicoClaw

```json
{
  "tools": {
    "skills": {
      "registries": {
        "github": {
          "enabled": true,
          "registry": "your-username/your-repo"
        }
      }
    }
  }
}
```

## Skill Format

Each skill should have a `SKILL.md` file with frontmatter:

```markdown
---
name: skill-name
description: What the skill does
---

# Skill Name

Your skill documentation here...
```

## Index JSON Format

The `skills-index.json` should look like:

```json
{
  "version": 1,
  "skills": [
    {
      "slug": "my-skill",
      "name": "My Skill",
      "description": "Does something useful",
      "path": "skills/my-skill"
    }
  ]
}
```
