# Development Guidelines

## Project Overview

- This is a Go-based demo project following standard Go project organization and practices
- Tech stack: Go language with standard library and approved third-party dependencies
- All code must be well-tested, maintainable, and follow Go idioms and best practices

## Project Architecture

### Directory Structure

- `/cmd/` - Main applications for this project, directory name should match executable name
- `/internal/` - Private application and library code not meant to be used by external applications
- `/pkg/` - Library code that's safe to use by external applications
- `/api/` - API protocol definitions, OpenAPI/Swagger specs, JSON schema files, etc.
- `/web/` - Web application specific components: static web assets, server side templates, SPAs
- `/configs/` - Configuration file templates or default configs
- `/test/` - Additional external test apps and test data
- `/docs/` - Design and user documents

### Module Management

- Maintain a clean `go.mod` file with explicit versioning for all dependencies
- Avoid dependency conflicts by carefully managing module requirements
- Document any non-standard dependency in comments within `go.mod`

## Coding Standards

### Naming Conventions

- Use camelCase for private variables, function names, and method names
- Use PascalCase for exported variables, function names, method names, and struct fields
- Use snake_case for file names and directory names
- Abbreviations should be consistent in their case (e.g., `URL`, not `Url`)

### Formatting

- Always run `gofmt` or `goimports` on code before committing
- Line length should be reasonable (aim for < 100 characters)
- Group imports in the standard Go way (standard library, third-party, project imports)

### Error Handling

- Always check errors explicitly
- Return errors rather than using panic
- Use custom error types for specific error conditions where appropriate
- Wrap errors with context using `fmt.Errorf("context: %w", err)` or equivalent

### Comments and Documentation

- All exported functions must have a proper documentation comment
- Complex logic should be explained with inline comments
- Use `// TODO:` or `// FIXME:` prefixes for temporary solutions or known issues

## Feature Implementation Guidelines

### Development Process

- For new features, first define interfaces and tests before implementation
- Ensure backward compatibility when modifying existing features
- Create small, focused PRs that are easy to review

### Testing

- Write unit tests for all functions and methods
- Integration tests should cover critical system interactions
- Test files should be named with `_test.go` suffix
- Aim for high test coverage particularly in core functionality

## Third-Party Library Usage

### Dependency Management

- Only add dependencies when absolutely necessary
- Prefer standard library solutions when available
- All dependencies must be added to `go.mod` with explicit versioning
- Document why a dependency is needed in commit messages

### Approved Libraries

- For HTTP: `net/http` (standard library) or `gorilla/mux`
- For database: `database/sql` with appropriate drivers
- For configuration: `viper` or similar configuration management library
- For CLI applications: `cobra` or similar command-line interface library

## Workflow Standards

### Git Practices

- Use feature branches for all changes
- Commit messages should be clear and descriptive
- Rebase feature branches against main before merge
- Delete branches after merge

### Checkpointing and Safe State

- Create regular, restorable checkpoints in git history by making atomic, self-contained commits and after completion of a task
- Ensure each commit and PR leaves the repository in a buildable and testable state
- Use descriptive PR titles and commit messages to clarify the purpose of each checkpoint
- Tag significant milestones or releases with annotated git tags

### Code Review

- All code must be reviewed before merging
- Address all comments before merging
- Ensure tests pass before requesting review
- Maintainers should enforce code quality standards

## Key File Interactions

### Configuration Management

- Configuration changes in `/configs/` require corresponding updates in application code
- New configuration options must be documented

### API Changes

- API changes in `/api/` require corresponding updates in handler implementations
- API changes must maintain backward compatibility when possible

## AI Decision Guidelines

### Code Modification Priority

1. Correctness - Code must work correctly
2. Readability - Code should be easy to understand
3. Maintainability - Code should be easy to maintain
4. Performance - Code should be efficient

### Decision Making Process

- When encountering ambiguity, follow existing patterns in the codebase
- When adding new code, follow Go idioms and standard practices
- When optimizing, prioritize readability unless performance is critical

## Prohibitions

- DO NOT use global variables for application state
- DO NOT ignore errors
- DO NOT use panic/recover as flow control
- DO NOT write overly complex functions (follow single responsibility principle)
- DO NOT hardcode configuration values, use configuration files instead
- DO NOT use deprecated libraries or functions
- DO NOT commit directly to main branch
- DO NOT add dependencies without justification