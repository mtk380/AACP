# Contributing to AACP

Thanks for your interest in improving AACP. We welcome bug fixes, documentation updates, and new features.

## Before You Start

- Check existing issues and pull requests to avoid duplicate work.
- For significant architecture or protocol changes, open an issue first to discuss the design.

## Development Setup

```bash
git clone https://github.com/mtk380/AACP.git
cd AACP
```

Optional (macOS + Homebrew):

```bash
./scripts/bootstrap_dev_tools.sh
```

## Common Commands

Run these before opening a pull request:

```bash
make fmt
make lint
make test
make e2e
```

If your changes involve Protobuf definitions:

```bash
make proto
```

## Pull Request Guidelines

- Keep PRs focused and small enough for review.
- Write clear commit messages (Conventional Commits are recommended, e.g. `feat: ...`, `fix: ...`).
- Update docs when behavior, APIs, or configuration changes.
- Add or update tests for new logic and bug fixes.
- For UI changes, include screenshots or short demo notes.

## Issue Reporting

When opening an issue, include:

- What you expected to happen.
- What actually happened.
- Reproduction steps and environment details.

## Security Policy

Please do **not** disclose security vulnerabilities publicly in issues.
Use GitHub's private vulnerability reporting flow (Security tab) to report them responsibly.

## License

By contributing, you agree that your contributions are licensed under the same terms as this repository's [MIT License](./LICENSE).
