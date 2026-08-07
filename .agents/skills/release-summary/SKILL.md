---
name: release-summary
description: Create a summary for the release
license: Maintenance
---

# Release Summary Skill

## Description
Generate a user-friendly release summary for the LazyAzure project that complements the auto-generated changelog from GoReleaser.

## Usage
The user will specify a release version (e.g., "v0.2.5" or "0.2.5"). You should:

1. Identify the previous version tag
2. Get all commits between the previous version and the specified version
3. Generate a concise, user-facing summary formatted as copyable markdown

## Steps

### 1. Get Version Information
```bash
git tag --list --sort=-version:refname | grep -E "^v?[0-9]\+\.[0-9]\+\.[0-9]\+$" | head -20
```

Find the specified version and the version immediately before it in the sorted list.

### 2. Get Commits Between Versions
```bash
git log --oneline <previous-tag>..<current-tag>
```

### 3. Analyze Conventional Commits
Parse the commits and categorize them:

**Commit Types:**
- `feat:` or `feat(scope):` → New features
- `fix:` or `fix(scope):` → Bug fixes
- `docs:` → Documentation (usually filtered out by goreleaser)
- `test:` → Tests (usually filtered out by goreleaser)
- `refactor:` → Code refactoring
- `chore:` → Maintenance tasks
- `perf:` → Performance improvements

### 4. Generate Summary Format

Create a concise summary with these sections:

```markdown
## What's New in vX.Y.Z

[Brief 1-2 sentence overview of the main changes]

### ✨ Highlights
- Key feature 1 (if any)
- Key feature 2 (if any)
- Notable bug fix or improvement

### 🐛 Bug Fixes
- Brief description of bug fix (user impact focused)
```

### 5. Guidelines for Writing

**DO:**
- Focus on user impact, not implementation details
- Use active voice and present tense
- Be concise (bullet points, not paragraphs)
- Mention specific features/benefits users will notice
- Include examples if helpful (e.g., "New keyboard shortcut X to do Y")

**DON'T:**
- Include internal refactoring or test changes
- Mention file names or internal code structure
- Use technical jargon without explanation
- Include every single commit (focus on important ones)

### 6. Output Format

**CRITICAL**: Always output the release summary wrapped in a **markdown code block** (triple backticks) so users can copy the literal markdown text. The content inside the code block should be valid markdown that can be pasted directly into GitHub releases or other documentation.

Format your output like this:

````
```markdown
## What's New in vX.Y.Z

[Your content here]
```
````

### 7. Example Output

For v0.2.5 (based on commits), output exactly this:

```markdown
## What's New in v0.2.5

This release improves navigation and filtering with better cursor management and alphabetical sorting.

### ✨ Highlights
- **Cursor preservation**: Cursor position is now maintained when refreshing data with `r`
- **Alphabetical sorting**: Subscriptions, resource groups, and resources are now sorted alphabetically for easier navigation
- **Subscription context**: Resource and resource group details now show the subscription name

### 🐛 Bug Fixes
- Fixed cursor jumping to top when filtering lists
```

## Implementation Notes

- The `.goreleaser.yml` already generates a detailed changelog with installation instructions
- This summary should be placed BEFORE the auto-generated changelog in the release notes
- Keep it short and focused on what users care about
- If there are no notable changes, keep it minimal (just the overview)
