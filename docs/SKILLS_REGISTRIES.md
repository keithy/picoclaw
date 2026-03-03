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
picoclaw skills install --registry index:angelhub self-config
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

### Index

Install skills from any HTTP endpoint that serves a skills-index.json.

**Basic Configuration:**

```json
{
  "tools": {
    "skills": {
      "registries": {
        "index:myorg": {
          "enabled": true,
          "index_url": "https://example.com/skills-index.json"
        }
      }
    }
  }
}
```

**Configuration with security options:**

```json
{
  "tools": {
    "skills": {
      "registries": {
        "index:angelhub": {
          "enabled": true,
          "index_url": "https://raw.githubusercontent.com/wiki/keithy/angelhub/skills-index.json",
          "extra_header": "X-Custom-Header: value",
          "authorization_header": "Bearer token",
          "agent_header": "picoclaw/1.0",
          "allowed_prefixes": [
            "https://raw.githubusercontent.com/wiki/keithy/angelhub/",
            "https://raw.githubusercontent.com/keithy/angelhub/"
          ]
        }
      }
    }
  }
}
```

## Creating Your Own Registry

To create a skill registry:

### 1. Create a Repository

Create a public repository to host your skills.

### 2. Add Skills

Add skills in the `picoclaw/skills/` directory (or `skills/` for ecosystem-agnostic). Each skill needs a `SKILL.md` file:

```
picoclaw/
└── skills/
    ├── self/
    │   ├── self-config/
    │   │   └── SKILL.md
    │   └── self-debug/
    │       └── SKILL.md
    └── weather/
        └── SKILL.md
```

### 3. Create the Index Workflow

Add a workflow to generate the skills index. See [AngelHub's workflow](https://github.com/keithy/angelhub/blob/main/.github/workflows/picoclaw-skills-index.yml) for a complete example.

### 4. Enable in PicoClaw

```json
{
  "tools": {
    "skills": {
      "registries": {
        "index:myorg": {
          "enabled": true,
          "index_url": "https://example.com/skills-index.json"
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
