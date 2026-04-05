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

<!-- brief:tools:start (generated by: brief list -readme tools) -->
## What it detects

54 language ecosystems with 446 tool definitions across 20 categories.

**Languages:** Ada, C, C#, C++, COBOL, Clojure, Common Lisp, Crystal, D, Dart, Deno, Elixir, Elm, Emacs Lisp, Erlang, F#, Fortran, GDScript, Gleam, Go, Groovy, Haskell, Haxe, Java, JavaScript, Julia, Kotlin, Lua, Mojo, Nim, Nix, OCaml, Objective-C, Odin, PHP, Perl, Prolog, Python, R, Racket, Roc, Ruby, Rust, Scala, Scheme, Solidity, Swift, Tcl, TypeScript, V, VHDL, Verilog, Zig.

**Package Managers:** Alire, Bun, Bundler, Cabal, Cargo, CocoaPods, Composer, Conan, Conda, DUB, Deno Modules, Gleam Packages, Go Modules, Gradle, Haxelib, Maven, Mix, Nix Flakes, NuGet, PDM, Pipenv, Pkg, Poetry, Quicklisp, Shards, Swift Package Manager, Yarn, dotnet CLI, elm, fpm, npm, opam, pip, pnpm, pub, rebar3, sbt, uv, vcpkg.

**Test:** AVA, Alcotest, Artillery, Bats, Bruno, Catch2, Cucumber, Cypress, EUnit, ExUnit, Gatling, Ginkgo, Google Test, Hspec, Hurl, JUnit, Jasmine, Jest, Kotest, Lighthouse CI, Locust, MSW, Minitest, Mocha, Newman, PHPUnit, Pest, Playwright, REST Client, RSpec, ScalaTest, Selenium, Testify, Testing Library, Vitest, XCTest, axe-core, benchmark-ips, cargo test, clojure.test, criterion, crystal spec, dart test, deno test, dotnet test, gleam test, go test, hyperfine, k6, kotlin.test, pytest, pytest-benchmark, tape, testament, zig test.

**Lint:** Ameba, Biome, Checkstyle, Clippy, Credo, ESLint, Flake8, HLint, Husky, Lefthook, Overcommit, PHP_CodeSniffer, PMD, Pylint, Revive, Roslyn Analyzers, RuboCop, Ruff, ShellCheck, SpotBugs, Stylelint, SwiftLint, Vale, WartRemover, actionlint, clang-tidy, clj-kondo, commitlint, cspell, dart analyze, deno lint, detekt, elvis, golangci-lint, hadolint, markdownlint, oxlint, pre-commit, typos.

**Format:** Black, Ormolu, PHP CS Fixer, Prettier, Spotless, StandardRB, SwiftFormat, clang-format, cljfmt, crystal tool format, dart format, deno fmt, dotnet format, dprint, erlfmt, gleam format, gofmt, google-java-format, isort, ktlint, mix format, nimpretty, ocamlformat, rustfmt, scalafmt, yapf, zig fmt.

**Typecheck:** Dialyxir, Dialyzer, Flow, PHPStan, Pyright, Sorbet, Steep, mypy, tsc.

**Docs:** Docsify, Docusaurus, Dokka, Doxygen, ExDoc, Hugo, Javadoc, Jekyll, MkDocs, Nextra, Read the Docs, Redoc, Sphinx, Starlight, Storybook, Swagger UI, TypeDoc, VitePress, Yard, cargo doc, dart doc, deno doc, mdBook, phpDocumentor, pkgsite.

**Build:** Actix Web, AdonisJS, Angular, Astro, Autotools, Axum, CMake, Chi, Django, Dune, Echo, Electron, Eleventy, Ember.js, Express, FastAPI, Fastify, Fiber, Flask, Flutter, Foundry, Gatsby, Gin, GoReleaser, Hardhat, Hono, Koa, Laravel, Less, Mage, Make, Meson, NestJS, Next.js, Nuxt, Parcel, Phoenix, PostCSS, Qwik, Rails, Rake, React, React Native, Remix, Rocket, Rollup, Rspack, SWC, Sass, Sinatra, SolidJS, Spring Boot, Svelte, SvelteKit, Symfony, Tailwind CSS, Tauri, UnoCSS, Vite, Vue, Webpack, cross, esbuild, tsup.

**Codegen:** Buf, GraphQL Code Generator, OpenAPI, Protobuf, Thrift, Wire, ent, gRPC, mockgen, sqlc.

**Database:** ActiveRecord, Alembic, Atlas, Dbmate, Diesel, Drizzle, Ecto, Flyway, GORM, Goose, Knex.js, Liquibase, MikroORM, Mongoose, Peewee, Prisma, SQLAlchemy, SQLite, Sequel, Sequelize, Tortoise ORM, TypeORM, pgx, sqlx.

**Security:** Bandit, Brakeman, Grype, OWASP Dependency-Check, SOPS, Semgrep, Snyk, SonarQube, Trivy, TruffleHog, bundler-audit, cargo-audit, govulncheck, npm audit, osv-scanner, pip-audit.

**CI:** Azure Pipelines, Buildkite, CircleCI, Cloudflare Workers, Dagger, Drone, Earthly, Fly.io, GitHub Actions, GitLab CI, Jenkins, Netlify, Render, Travis CI, Vercel.

**Container:** Cloud Native Buildpacks, Dev Container, Docker, Docker Compose, Podman.

**Infra:** AWS CDK, Ansible, CloudFormation, Helm, Kubernetes, Kustomize, Packer, Pulumi, Serverless Framework, Terraform, Vagrant.

**Monorepo:** Bazel, Cargo workspaces, Go workspace, Lerna, Moon, Nx, Pants, Rush, Turborepo, Yarn workspaces, pnpm workspaces.

**Environment:** Flipper, JetBrains IDE, LaunchDarkly, Mise, Unleash, VS Code, Volta, asdf, direnv, dotenv, pyenv.

**i18n:** Crowdin, Fluent, FormatJS, Lingui, Rails i18n, Transifex, gettext, i18next, vue-i18n.

**Release:** Changesets, cargo-release, conventional-changelog, git-cliff, np, release-please, semantic-release, standard-version, twine.

**Coverage:** Codecov, Coveralls, Excoveralls, JaCoCo, Sentry, SimpleCov, c8, cargo-tarpaulin, coverage.py, go test -cover, nyc.

**Dep Updates:** Dependabot, Git Submodules, Renovate.

Run `brief list tools` for the full list.
<!-- brief:tools:end -->

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
