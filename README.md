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

Test:        go test (go test ./...)
Lint:        golangci-lint (golangci-lint run)  [.golangci.yml]
Format:      gofmt (gofmt -w .)
Typecheck:   —
Docs:        —
Build:       —
Security:    —
CI:          GitHub Actions  [.github/workflows/]
Container:   —
Dep Updates: Dependabot  [.github/dependabot.yml]

Style:       tabs (inferred)  LF
Layout:      internal/ cmd/

             OS: ubuntu-latest, macos-latest, windows-latest (CI matrix)

Resources:   README.md
Resources:   LICENSE (MIT)

Git:         branch add-commit-statuses (default: main)  58 commits
             origin: git@github.com:git-pkgs/forge.git

Lines:       22912 code  191 files (scc)

164.0ms  184 files checked  7/230 tools matched
```

Use `--verbose` to include homepage, docs, and repo links for each detected tool.

## Enrichment

`brief enrich` runs the same scan, then fetches metadata from external APIs for each direct dependency. The output gains an `enrichment` section with downloads, dependents, security advisories, runtime end-of-life status, and OpenSSF Scorecard.

```
brief enrich .
brief enrich --json .
brief enrich --verbose .
```

Data sources: [ecosyste.ms](https://ecosyste.ms) for package metadata, [endoflife.date](https://endoflife.date) for runtime lifecycle, [OpenSSF Scorecard](https://securityscorecards.dev) for repo security.

## How it works

Detection rules are data, not code. Each tool is defined in a TOML file under `knowledge/`, organized by ecosystem:

```
knowledge/
├── ruby/       language, bundler, rspec, minitest, rubocop, sorbet
├── python/     language, pip, uv, poetry, pipenv, pdm, pytest, ruff, mypy, black
├── go/         language, gomod, gotest, golangci-lint, gofmt
├── node/       language, typescript, npm, pnpm, yarn, bun, jest, vitest, eslint, prettier, biome
├── rust/       language, cargo, clippy, rustfmt
├── java/       language, maven, gradle, junit, checkstyle, spotbugs
├── elixir/     language, mix, exunit, credo, dialyzer
├── php/        language, composer, phpunit, phpstan, php-cs-fixer
├── deno/       language, deno
├── gleam/      language, gleam
├── nix/        language, flakes
├── + csharp, swift, kotlin, haskell, scala, dart, crystal, julia
├── + clojure, r, lua, perl, zig, nim, d, elm, haxe, conda, cocoapods
└── _shared/    github-actions, gitlab-ci, docker, devcontainer, dependabot, renovate, pre-commit
```

A tool definition looks like this:

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

Adding support for a new tool is a single TOML file. No Go code changes needed.

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

## License

MIT
