# brief

A single-binary CLI tool that detects a software project's toolchain, configuration, and conventions, then outputs a structured report. Written in Go, 30 ecosystems, 230 tool definitions.

brief answers the bootstrap questions every AI coding agent, new contributor, and CI pipeline faces: what language is this, how do I install dependencies, how do I run the tests, what linter is configured.

It does not score, grade, or judge. It reports facts.

## Install

```
go install github.com/git-pkgs/brief/cmd/brief@latest
```

Or download a binary from [releases](https://github.com/git-pkgs/brief/releases).

## Usage

```
brief [flags] [path | url]        Detect project toolchain
brief enrich [flags] [path]       Detect and enrich with external data
brief list tools                  All tools in the knowledge base
brief list ecosystems             Supported ecosystems
brief schema                      JSON output schema
```

Works on local paths, git URLs, and registry packages:

```
brief .                                       Local directory
brief /path/to/project                        Any local path
brief https://github.com/expressjs/express    Git URL (cloned to temp dir)
brief npm:express                             Registry package (resolved to source repo)
brief gem:rails
brief crate:serde
brief pypi:requests
```

Remote sources are shallow-cloned by default. Use `--depth 0` for a full clone, `--keep` to preserve the clone, or `--dir ./somewhere` to clone into a specific directory.

JSON when piped, human-readable on a TTY. Force either with `--json` or `--human`. Use `--category test` to filter to a single category.

```
brief dev — /home/user/forge

Language:        Go
Package Manager: Go Modules (go mod download)
                 Lockfile: go.sum
                 7 runtime (222 total)

Test:        go test (go test ./...)
Lint:        golangci-lint (golangci-lint run)  [.golangci.yml]
Format:      gofmt (gofmt -w .)
Build:       GoReleaser (goreleaser release --clean)  [.goreleaser.yml]
CI:          GitHub Actions  [.github/workflows/]
Dep Updates: Dependabot  [.github/dependabot.yml]

Style:       tabs (inferred)  LF
Layout:      internal/ cmd/

             OS: ubuntu-latest, macos-latest, windows-latest (CI matrix)

Resources:   README.md
Resources:   LICENSE (MIT)

Git:         branch main  58 commits
             origin: git@github.com:git-pkgs/forge.git

Lines:       22912 code  191 files (scc)

112.0ms  311 files checked  8/230 tools matched
```

Use `--verbose` to include homepage, docs, and repo links for each detected tool.

## Enrichment

`brief enrich` runs the same scan, then fetches metadata from external APIs about the project itself: what packages it publishes, their download counts and dependents, runtime end-of-life status, and OpenSSF Scorecard.

```
brief enrich .
brief enrich --json .
brief enrich --verbose .
```

Data sources: [ecosyste.ms](https://ecosyste.ms) for published package metadata, [endoflife.date](https://endoflife.date) for runtime lifecycle, [OpenSSF Scorecard](https://securityscorecards.dev) for repo security.

## What it detects

30 language ecosystems with 230 tool definitions across 18 categories.

**Languages (30):** Go, Ruby, Python, JavaScript, TypeScript, Rust, Java, Kotlin, Scala, Elixir, PHP, Swift, C#, Dart, Haskell, Clojure, Crystal, Julia, Nim, Zig, Lua, Perl, R, D, Elm, Gleam, Haxe, Nix, Deno, plus CocoaPods and Conda ecosystems.

**Test (27):** go test, Jest, Vitest, RSpec, Minitest, pytest, JUnit, PHPUnit, ExUnit, cargo test, Playwright, Cypress, Cucumber, Selenium, k6, Locust, Artillery, criterion, axe-core, Lighthouse CI, and more.

**Lint (19):** golangci-lint, ESLint, RuboCop, Ruff, Clippy, Biome, Stylelint, commitlint, hadolint, ShellCheck, markdownlint, Semgrep, pre-commit, Lefthook, Husky, and more.

**Format (11):** gofmt, Prettier, Black, rustfmt, isort, dprint, scalafmt, ktlint, SwiftFormat, StandardRB, PHP CS Fixer.

**Build (29):** Webpack, Vite, esbuild, Rollup, Parcel, tsup, GoReleaser, Mage, Rake, Tailwind CSS, PostCSS, Sass, plus framework detection for Rails, Django, FastAPI, Express, Fastify, Gin, Phoenix, Spring Boot, Actix, Next.js, Nuxt, Astro, Gatsby, SvelteKit, Eleventy.

**Database (14):** ActiveRecord, Prisma, Alembic, Diesel, Ecto, Flyway, Liquibase, Goose, Dbmate, Drizzle, TypeORM, Sequelize, SQLAlchemy, GORM.

**Codegen (6):** Protobuf, Buf, OpenAPI, GraphQL Code Generator, ent, sqlc.

**Infrastructure (7):** Terraform, Pulumi, Ansible, Kubernetes, Helm, AWS CDK, Serverless Framework.

**CI/Deployment (6):** GitHub Actions, GitLab CI, Earthly, Dagger, Vercel, Netlify.

**Container (3):** Docker, Docker Compose, Dev Container.

**Monorepo (7):** Nx, Turborepo, Bazel, Pants, Lerna, pnpm workspaces, Go workspaces.

**Release (6):** semantic-release, release-please, cargo-release, Changesets, git-cliff, conventional-changelog.

**i18n (5):** i18next, gettext, Rails i18n, Crowdin, Transifex.

**Also:** package managers (31), type checkers (4), docs generators (8), security tools (3), coverage services (3), dependency update bots (3), environment tools (9).

Run `brief list tools` for the full list.

## How it works

Detection rules are data, not code. Each tool is defined in a TOML file under `knowledge/`, organized by ecosystem. Adding a new tool is a single TOML file with no Go code changes.

```toml
[tool]
name = "RSpec"
category = "test"
homepage = "https://rspec.info"
description = "BDD testing framework for Ruby"

[detect]
files = ["spec/", ".rspec"]
dependencies = ["rspec", "rspec-core"]

[commands]
run = "bundle exec rspec"
alternatives = ["rake spec", "rspec"]

[config]
files = [".rspec", "spec/spec_helper.rb"]
```

Detection uses five primitives: file/directory presence, dependency names from parsed manifests, file content matching, structured key existence (JSON/TOML), and ecosystem filtering to prevent cross-language false positives.

## Library usage

The detection engine, knowledge base, and reporters are separate Go packages. Import them directly instead of shelling out to the binary:

```go
import (
    "github.com/git-pkgs/brief"
    "github.com/git-pkgs/brief/kb"
    "github.com/git-pkgs/brief/detect"
    "github.com/git-pkgs/brief/report"
)

knowledgeBase, _ := kb.Load(brief.KnowledgeFS)
engine := detect.New(knowledgeBase, "/path/to/project")
r, _ := engine.Run()
report.JSON(os.Stdout, r)
```

## Contributing

Adding a new tool: create a TOML file in the appropriate ecosystem directory under `knowledge/`, add test fixtures in `testdata/`, run `go test ./...`.

Adding a new ecosystem: create a directory under `knowledge/`, add `language.toml` plus at least a package manager, test framework, and linter.

See [CONTRIBUTING.md](CONTRIBUTING.md) for detection primitives and category definitions.

## License

MIT
