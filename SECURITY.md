# Security Policy

## Supported Versions

Security fixes are applied to the latest released minor version.

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅        |
| < 0.1   | ❌        |

## Reporting a Vulnerability

Please report suspected vulnerabilities privately via
[GitHub Security Advisories](https://github.com/gourdian25/grlog/security/advisories/new)
rather than opening a public issue.

Include:

- A description of the issue and its impact
- Steps or a proof-of-concept to reproduce
- Affected version(s)

You can expect an acknowledgment within a week. Once a fix is available, the
advisory will be published together with a patched release.

## Scope Notes

grlog is a logging library with no network listeners and no dependencies
outside the Go standard library. The most relevant security considerations
for users are:

- **Log injection**: the JSON formatter escapes all field keys and values;
  entry fields can never overwrite entry metadata (`timestamp`, `level`,
  `message`, `caller`). The plain-text formatter does not escape newlines —
  prefer JSON output when logs are consumed by other systems.
- **Sensitive data**: the library does not redact values; do not log secrets.
- **File permissions**: log files are created with mode 0644 and directories
  with 0755 so operators and log shippers can read them. Restrict the parent
  directory if your logs are sensitive.
