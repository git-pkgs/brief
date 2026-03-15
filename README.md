# brief

A single-binary CLI tool that detects a software project's toolchain, configuration, and conventions, then outputs a structured report. Written in Go.

brief answers the bootstrap questions every AI coding agent, new contributor, and CI pipeline faces: what language is this, how do I install dependencies, how do I run the tests, what linter is configured.

It does not score, grade, or judge. It reports facts.

## Install

```
go install github.com/git-pkgs/brief/cmd/brief@latest
```

Or download a binary from [releases](https://github.com/git-pkgs/brief/releases).

## Usage

```
brief .
```

JSON when piped, human-readable on a TTY. Force either with `--json` or `--human`.

```
brief dev — /home/user/24pullrequests

Language:        Ruby
Package Manager: Bundler (bundle install)

Test:        Minitest (bundle exec rake test)
             RSpec (bundle exec rspec)  [.rspec, spec/spec_helper.rb, spec/rails_helper.rb]
Lint:        RuboCop (bundle exec rubocop)  [.rubocop.yml]
Format:      —
Typecheck:   —
Docs:        —
Build:       —
Security:    —
CI:          GitHub Actions  [.github/workflows/]
Container:   Docker  [Dockerfile, docker-compose.yml]
Dep Updates: Dependabot  [.github/dependabot.yml]

Layout:      lib/ app/  test/ spec/

Runtime:     .ruby-version: 4.0.1

Resources:   Readme.md
Resources:   CONTRIBUTING.md
Resources:   LICENSE

2.3ms  107 files checked  8/30 tools matched
```

Use `--verbose` to include homepage, docs, and repo links for each detected tool.

## How it works

Detection rules are data, not code. Each tool is defined in a TOML file under `knowledge/`, organized by ecosystem:

```
knowledge/
├── ruby/       language.toml, bundler.toml, rspec.toml, rubocop.toml, ...
├── python/     language.toml, uv.toml, pytest.toml, ruff.toml, mypy.toml, ...
├── go/         language.toml, gomod.toml, gotest.toml, golangci-lint.toml
├── node/       language.toml, npm.toml, jest.toml, eslint.toml, ...
├── rust/       language.toml, cargo.toml, clippy.toml, rustfmt.toml
└── _shared/    github-actions.toml, docker.toml, dependabot.toml, ...
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
