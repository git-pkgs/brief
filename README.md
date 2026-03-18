# brief

A single-binary CLI tool that detects a software project's toolchain, configuration, and conventions, then outputs a structured report. Written in Go, 54 ecosystems, 355 tool definitions.

brief answers the bootstrap questions every AI coding agent, new contributor, and CI pipeline faces: what language is this, how do I install dependencies, how do I run the tests, what linter is configured.

It does not score, grade, or judge. It reports facts.

## Use with AI coding agents

Add this to your `CLAUDE.md`, `agents.md`, or equivalent agent instructions file:

```
Before starting work on this project, run `brief .` to understand the toolchain,
test commands, linters, and project conventions. If on a branch, also run
`brief diff` to see which parts of the toolchain are affected by your changes.
```

The agent will get back structured information about the project's language, package manager, test runner, linter, formatter, build tools, and more, so it doesn't have to guess or ask you. On a feature branch, `brief diff` narrows that down to just the tools relevant to what's been changed, so the agent knows which linters to run, which test frameworks matter, and which config files are in play.

To let Claude Code run `brief` without prompting for approval each time, add this to `~/.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(brief *)"]
  }
}
```

## Install

```
go install github.com/git-pkgs/brief/cmd/brief@latest
```

Or download a binary from [releases](https://github.com/git-pkgs/brief/releases).

## Usage

```
brief [flags] [path | url]        Detect project toolchain
brief diff [flags] [ref1] [ref2]  Detect only what changed between refs
brief missing [flags] [path]      Show recommended tooling gaps
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
brief dev — /home/user/myproject

Language:        Go
Package Manager: Go Modules (go mod download)
                 Lockfile: go.sum
                 9 runtime (223 total)

Test:        go test (go test ./...)
Lint:        golangci-lint (golangci-lint run)  [.golangci.yml]
Format:      gofmt (gofmt -w .)
Docs:        pkgsite (go run golang.org/x/pkgsite/cmd/pkgsite@latest)
Build:       GoReleaser (goreleaser release --clean)  [.goreleaser.yaml]
Security:    govulncheck (govulncheck ./...)
CI:          GitHub Actions  [.github/workflows/]
Coverage:    go test -cover (go test -coverprofile=coverage.out ./...)
Dep Updates: Dependabot  [.github/dependabot.yml]

Style:       tabs (inferred)  LF
Layout:      cmd/

             OS: ubuntu-latest, macos-latest, windows-latest (CI matrix)

Resources:   README.md
Resources:   CONTRIBUTING.md
Resources:   LICENSE (MIT)

Git:         branch main  71 commits
             origin: git@github.com:user/myproject.git

Lines:       9295 code  397 files (scc)

148.7ms  428 files checked  11/355 tools matched
```

Use `--verbose` to include homepage, docs, and repo links for each detected tool.

## Diff

`brief diff` runs the same detection but filters the report to only show tools, languages, and configuration relevant to files that changed. Useful for understanding what a branch or PR touches in terms of toolchain.

```
brief diff                        Compare against default branch + uncommitted
brief diff main                   Compare main to HEAD + uncommitted
brief diff v1.0.0 v2.0.0         Compare between two refs
```

With no arguments it auto-detects the default branch from `origin/HEAD`, falling back to `main` or `master`. The output lists changed files and only the toolchain entries those files relate to: if you changed a `.go` file, you'll see Go and its tools but not Python. If you changed `.golangci.yml`, you'll see golangci-lint. If you changed `go.mod`, you'll see dependency information.

Same output format as `brief` -- JSON when piped, human-readable on a TTY.

## Missing

`brief missing` checks which recommended tool categories are absent for the project's detected ecosystems. It compares what's detected against five categories every project benefits from: test, lint, format, typecheck, and docs.

```
brief missing .
brief missing --json .
```

For each gap it suggests the canonical tool for that ecosystem, with the command to run and a link to docs.

```
Detected: python

Missing recommended tooling:

  Test         No test tool configured
               Suggested: pytest (pytest)
               https://docs.pytest.org

  Lint         No lint tool configured
               Suggested: Ruff (ruff check .)
               https://docs.astral.sh/ruff/

  Format       No format tool configured
               Suggested: Black (black .)
               https://black.readthedocs.io/en/stable/
```

Tools built into the language runtime (go test, gofmt, cargo clippy, dart analyze, deno lint, etc.) are detected automatically when the language is present and won't show as missing.

## Enrichment

`brief enrich` runs the same scan, then fetches metadata from external APIs about the project itself: what packages it publishes, their download counts and dependents, runtime end-of-life status, and OpenSSF Scorecard.

```
brief enrich .
brief enrich --json .
brief enrich --verbose .
```

Data sources: [ecosyste.ms](https://ecosyste.ms) for published package metadata, [endoflife.date](https://endoflife.date) for runtime lifecycle, [OpenSSF Scorecard](https://securityscorecards.dev) for repo security.

## What it detects

54 language ecosystems with 355 tool definitions across 20 categories.

**Languages (54):** Go, Ruby, Python, JavaScript, TypeScript, Rust, Java, Kotlin, Scala, Elixir, PHP, Swift, C#, Dart, Haskell, Clojure, Crystal, Julia, Nim, Zig, Lua, Perl, R, D, Elm, Gleam, Haxe, Nix, Deno, C, C++, Objective-C, Erlang, OCaml, F#, Groovy, Solidity, GDScript, Fortran, COBOL, Ada, VHDL, Verilog, Mojo, Roc, V, Odin, Scheme, Racket, Prolog, Tcl, Common Lisp, Emacs Lisp, plus CocoaPods and Conda ecosystems.

**Test (46):** go test, Jest, Vitest, Mocha, AVA, RSpec, Minitest, pytest, JUnit, PHPUnit, ExUnit, EUnit, cargo test, Google Test, Catch2, Playwright, Cypress, MSW, Locust, Artillery, axe-core, Lighthouse CI, XCTest, dart test, deno test, kotlin.test, ScalaTest, crystal spec, Hspec, Alcotest, zig test, gleam test, and more.

**Lint (31):** golangci-lint, ESLint, RuboCop, Ruff, Clippy, clang-tidy, Biome, Stylelint, commitlint, hadolint, ShellCheck, markdownlint, Semgrep, pre-commit, Lefthook, Husky, detekt, PHP_CodeSniffer, SwiftLint, Credo, HLint, elvis, clj-kondo, dart analyze, deno lint, WartRemover, Ameba, and more.

**Format (25):** gofmt, Prettier, Black, rustfmt, isort, clang-format, ocamlformat, dprint, scalafmt, ktlint, SwiftFormat, StandardRB, PHP CS Fixer, mix format, erlfmt, Ormolu, dart format, deno fmt, dotnet format, zig fmt, crystal tool format, cljfmt, nimpretty, gleam format, google-java-format.

**Build (48):** Webpack, Vite, esbuild, Rollup, Parcel, tsup, Rspack, GoReleaser, Mage, Rake, CMake, Make, Meson, Autotools, Hardhat, Foundry, Tailwind CSS, PostCSS, Sass, Less, UnoCSS, plus framework detection for Rails, Django, FastAPI, Express, Fastify, Koa, Hono, NestJS, AdonisJS, Gin, Phoenix, Spring Boot, Actix, Next.js, Nuxt, Remix, Angular, Ember.js, SolidJS, Qwik, Astro, Gatsby, SvelteKit, Eleventy.

**Database (15):** ActiveRecord, Prisma, Alembic, Diesel, Ecto, Flyway, Liquibase, Goose, Dbmate, Drizzle, TypeORM, Sequelize, SQLAlchemy, GORM, SQLite.

**Codegen (6):** Protobuf, Buf, OpenAPI, GraphQL Code Generator, ent, sqlc.

**Infrastructure (7):** Terraform, Pulumi, Ansible, Kubernetes, Helm, AWS CDK, Serverless Framework.

**CI/Deployment (7):** GitHub Actions, GitLab CI, Earthly, Dagger, Cloudflare Workers, Vercel, Netlify.

**Container (3):** Docker, Docker Compose, Dev Container.

**Monorepo (8):** Nx, Turborepo, Rush, Bazel, Pants, Lerna, pnpm workspaces, Go workspaces.

**Release (6):** semantic-release, release-please, cargo-release, Changesets, git-cliff, conventional-changelog.

**i18n (5):** i18next, gettext, Rails i18n, Crowdin, Transifex.

**Docs (18):** pkgsite, Sphinx, MkDocs, Javadoc, Docusaurus, VitePress, Storybook, Jekyll, Doxygen, ExDoc, Dokka, phpDocumentor, cargo doc, dart doc, deno doc, Docsify, Starlight, and more.

**Security (9):** govulncheck, npm audit, pip-audit, bundler-audit, cargo-audit, OWASP Dependency-Check, Snyk, Socket, and more.

**Coverage (9):** go test -cover, coverage.py, SimpleCov, JaCoCo, c8, cargo-tarpaulin, Codecov, Coveralls, and more.

**Also:** package managers (39), type checkers (6), dependency update bots (3), environment tools (9).

Run `brief list tools` for the full list.

## How it works

Detection rules are data, not code. Each tool is defined in a TOML file under `knowledge/`, organized by ecosystem. Adding a new tool is a single TOML file with no Go code changes.

```toml
[tool]
name = "RSpec"
category = "test"
default = true
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

The `default = true` flag marks a tool as the canonical choice for its category in that ecosystem. The `brief missing` command uses this to suggest the right tool when a category is absent.

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
