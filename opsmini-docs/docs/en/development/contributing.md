# Contributing Guide

Thank you for your interest in and contributions to OpsMini!

## Ways to Contribute

- **Report bugs**: submit on GitHub Issues with reproduction steps, environment information, and logs
- **Feature suggestions**: describe the use case and expectations in Issues
- **Submit code**: fork the repository → create a branch → commit → open a Pull Request
- **Improve documentation**: fix documentation errors and add usage examples

## Commit Conventions

- One PR focuses on one change; avoid doing everything at once
- Follow the official Go code style (`gofmt` / `go vet`)
- Add test cases for new features
- Commit messages should clearly describe "what was done" and "why"

## Directory Conventions

- Layers: `api/v1` (Controller) → `service` (business) → `repository` (data)
- Sensitive fields (secrets / tokens) are encrypted before storage
- New interfaces follow the unified response `{ code, message, data }`

## License

The project uses Apache License 2.0; contributing code means you agree to license it under this license.
