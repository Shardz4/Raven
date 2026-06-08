# 🪶 Contributing to Raven

First off, **thank you** for considering contributing to Raven! Every contribution — whether it's a bug report, feature suggestion, documentation improvement, or code patch — helps make autonomous AI development better for everyone.

This document provides guidelines and best practices for contributing to the project. Please read it before submitting your first pull request.

---

## 📋 Table of Contents

- [Code of Conduct](#-code-of-conduct)
- [Getting Started](#-getting-started)
- [Development Environment Setup](#-development-environment-setup)
- [Project Architecture](#-project-architecture)
- [How to Contribute](#-how-to-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Features](#suggesting-features)
  - [Submitting Pull Requests](#submitting-pull-requests)
- [Coding Standards](#-coding-standards)
- [Commit Convention](#-commit-convention)
- [Pull Request Process](#-pull-request-process)
- [Where to Start](#-where-to-start)
- [Community](#-community)
- [License](#-license)

---

## 🤝 Code of Conduct

By participating in this project, you agree to be respectful and constructive. We are committed to providing a welcoming and harassment-free environment for everyone, regardless of experience level, background, or identity.

**In short:**
- Be kind and courteous
- Respect differing viewpoints and experiences
- Accept constructive criticism gracefully
- Focus on what's best for the project and community

---

## 🚀 Getting Started

1. **Fork** the repository on GitHub
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<your-username>/Raven.git
   cd Raven
   ```
3. **Add the upstream remote:**
   ```bash
   git remote add upstream https://github.com/Shardz4/Raven.git
   ```
4. **Create a branch** for your work:
   ```bash
   git checkout -b feature/your-feature-name
   ```

---

## 🛠️ Development Environment Setup

### Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| **Go** | 1.25+ | Backend compilation |
| **Python** | 3.9+ | Streamlit frontend |
| **Docker Desktop** | Latest | Sandbox verification |
| **Git** | Latest | Version control |

### Backend Setup

```bash
# Navigate to the backend directory
cd backend

# Download Go dependencies
go mod download

# Copy and configure environment
cp ../.env.example ../.env
# Edit ../.env — add at least one LLM API key

# Build the binary
go build -o raven.exe .

# Run in monolithic mode
./raven.exe
```

### Frontend Setup

```bash
# From the project root
pip install -r requirements.txt

# Start the Streamlit UI
streamlit run app.py
```

### Docker Sandbox

```bash
# Build the sandbox image (required for patch verification)
docker build -t raven-sandbox:latest sandbox_env/
```

### Running Tests

```bash
cd backend
go test ./... -v
```

### Distributed Mode (Optional)

If you're working on the multi-agent architecture:

```bash
# Start the full distributed cluster
docker compose up --build
```

---

## 🏗️ Project Architecture

Understanding the codebase structure will help you contribute effectively. Raven has three main layers:

```
Raven/
├── app.py                 # Streamlit frontend (Python)
├── backend/               # Go backend (all core logic)
│   ├── main.go            # Monolithic entry point
│   ├── api/               # REST API + SSE handlers
│   ├── agents/            # Distributed agent entry points (7 agents)
│   ├── broker/            # NATS JetStream messaging
│   ├── bots/              # Telegram + Discord integrations
│   ├── config/            # Centralised configuration
│   ├── consensus/         # 🧠 RavenMind consensus engine
│   ├── github/            # Issue fetcher + Auto PR creator
│   ├── llm/               # LLM provider adapters (6 providers)
│   ├── sandbox/           # Docker sandbox execution
│   ├── store/             # SQLite persistence layer
│   └── validation/        # Safety gate + AST fingerprinting
├── docker-compose.yml     # Distributed deployment
└── sandbox_env/           # Docker image for test sandbox
```

### Key Packages to Know

| Package | What It Does | Good For |
|---|---|---|
| `llm/` | Abstracts LLM providers behind a common `Provider` interface | Adding new LLM providers |
| `consensus/` | RavenMind 4-phase scoring pipeline | Improving scoring algorithms |
| `validation/` | Safety checks + structural fingerprinting | Adding language validators |
| `sandbox/` | Docker container lifecycle for patch testing | Multi-language test support |
| `api/` | HTTP endpoints + SSE streaming | New API features |
| `bots/` | Telegram and Discord integrations | New bot platforms |
| `agents/` | Distributed agent entry points | Scaling architecture |
| `store/` | SQLite persistence + HTTP client | Data model changes |

---

## 💡 How to Contribute

### Reporting Bugs

Found a bug? Please [open an issue](https://github.com/Shardz4/Raven/issues/new) with:

- **Title**: A clear, concise description
- **Environment**: OS, Go version, Docker version
- **Steps to Reproduce**: Numbered steps to trigger the bug
- **Expected Behavior**: What should happen
- **Actual Behavior**: What actually happens
- **Logs**: Relevant console output or error messages (redact API keys!)
- **Screenshots**: If applicable (especially for frontend issues)

### Suggesting Features

Have an idea? We'd love to hear it! Open an issue with the `enhancement` label and include:

- **Problem Statement**: What problem does this solve?
- **Proposed Solution**: How should it work?
- **Alternatives Considered**: Other approaches you've thought about
- **Additional Context**: Mockups, diagrams, or references

### Submitting Pull Requests

Ready to code? Follow these steps:

1. **Check existing issues** — see if someone is already working on it
2. **Comment on the issue** — let others know you're picking it up
3. **Keep it focused** — one feature or fix per PR
4. **Write tests** — if applicable
5. **Update docs** — if your change affects the README or API
6. **Follow the coding standards** below

---

## 📝 Coding Standards

### Go (Backend)

- **Formatting**: Run `gofmt` before committing. All code must be `gofmt`-compliant.
- **Linting**: Run `go vet ./...` to catch common issues.
- **Naming**: Follow Go naming conventions — exported names are `PascalCase`, unexported are `camelCase`.
- **Error Handling**: Always check and handle errors. Never use `_` to discard errors silently in production paths.
- **Comments**: Exported functions and types must have godoc comments starting with the name.
- **Packages**: Keep packages focused. One responsibility per package.
- **Dependencies**: Discuss new dependencies in the PR — we prefer the standard library where possible.

```go
// ✅ Good
// ValidateGoCode parses Go source code and rejects patches with syntax errors.
func ValidateGoCode(code string) *Result {
    fset := token.NewFileSet()
    _, err := parser.ParseFile(fset, "patch.go", code, parser.AllErrors)
    if err != nil {
        return &Result{OK: false, Reason: "Go syntax error: " + err.Error()}
    }
    return &Result{OK: true, Reason: "OK"}
}

// ❌ Bad — no comment, error ignored, unclear naming
func check(c string) bool {
    _, _ = parser.ParseFile(token.NewFileSet(), "", c, 0)
    return true
}
```

### Python (Frontend)

- **Style**: Follow PEP 8.
- **Type Hints**: Use type hints for function signatures when practical.
- **Docstrings**: Add docstrings to public functions.

### General

- **No hardcoded secrets** — use environment variables via `.env`
- **No large binary files** — use `.gitignore` appropriately
- **Keep PRs small** — smaller changes are easier to review and merge

---

## 📦 Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/) for clear, parseable commit history:

```
<type>(<scope>): <short description>

[optional body]

[optional footer]
```

### Types

| Type | When to Use |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, no code change |
| `refactor` | Code restructuring, no feature change |
| `test` | Adding or updating tests |
| `chore` | Build process, CI, tooling |
| `perf` | Performance improvement |

### Examples

```
feat(llm): add Google Gemini provider adapter
fix(sandbox): handle timeout correctly on Windows
docs(readme): update quickstart for distributed mode
refactor(consensus): extract scoring logic into helper functions
test(validation): add safety gate tests for Rust code
chore(docker): upgrade base image to golang:1.25
```

### Scope Reference

Use these scopes to indicate which part of the codebase is affected:

`llm`, `consensus`, `sandbox`, `validation`, `api`, `bots`, `broker`, `agents`, `store`, `config`, `github`, `frontend`, `docker`, `ci`

---

## 🔄 Pull Request Process

1. **Sync with upstream** before starting:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Create a descriptive PR** with:
   - A clear title following the commit convention
   - A description explaining **what** changed and **why**
   - Links to related issues (use `Closes #123` to auto-close)
   - Screenshots or recordings for UI changes

3. **PR Checklist** — make sure you've done these before requesting review:
   - [ ] Code compiles without errors (`go build ./...`)
   - [ ] All existing tests pass (`go test ./...`)
   - [ ] New code is formatted (`gofmt`)
   - [ ] New code passes vet (`go vet ./...`)
   - [ ] Documentation is updated if needed
   - [ ] Commit messages follow the convention
   - [ ] No secrets, API keys, or personal data in the diff

4. **Review Process**:
   - A maintainer will review your PR
   - Feedback may be given — this is normal and collaborative
   - Once approved, a maintainer will merge your PR
   - Squash merging is preferred for clean history

---

## 🎯 Where to Start

Not sure what to work on? Here are some great first contributions:

### 🟢 Good First Issues

- Add input validation to the Streamlit frontend
- Improve error messages in LLM provider adapters
- Add more unit tests for the `validation/` package
- Fix typos or improve documentation

### 🟡 Intermediate

- Add a new LLM provider (e.g., Google Gemini, Mistral, Cohere)
- Add a safety validator for a new language (JavaScript, Rust, TypeScript)
- Improve the structural fingerprinting algorithm for non-Python code
- Add more test script templates in `sandbox/docker.go`
- Create a `/history` command for the Telegram/Discord bots

### 🔴 Advanced

- Implement caching for repeated LLM prompts
- Add WebSocket support alongside SSE for the streaming API
- Build a more sophisticated AST parser for Phase 3 (structural similarity)
- Implement horizontal scaling for the sandbox agent with a worker pool
- Add OpenTelemetry tracing across the distributed agent pipeline
- Create a GitHub Actions workflow for CI/CD

### Labels to Watch

Look for issues tagged with:
- `good first issue` — Great for newcomers
- `help wanted` — We'd appreciate community help
- `enhancement` — Feature requests open for implementation
- `bug` — Known issues that need fixing

---

## 🌐 Community

- **GitHub Issues**: For bug reports and feature requests
- **GitHub Discussions**: For questions, ideas, and general chat
- **Pull Requests**: For code contributions

When in doubt, open an issue first to discuss your idea before writing code. This saves everyone time and ensures your contribution aligns with the project's direction.

---

## 📄 License

By contributing to Raven, you agree that your contributions will be licensed under the [BSD 3-Clause License](LICENSE), the same license that covers the project.

---

**Thank you for helping make Raven better! 🪶**
