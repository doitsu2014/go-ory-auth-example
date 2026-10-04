# Code Generation Notes

- Every diagram step was checked against the implementation: endpoints
  (`/v1/auth/identifiers`, `/internal/hooks/kratos/{pre-registration,after-registration,after-login,courier}`),
  Transit paths and key names, vault table and columns, message ids
  4049001/4049002/4000007, courier outcomes, `/v1/me` lazy bind, admin
  lookup/reveal audit actions.
- Mermaid was validated with `@mermaid-js/mermaid-cli@11` (`mmdc`). All 5
  diagrams render. Two syntax issues were fixed:
  - `;` in a message ends the statement;
  - `#` starts an entity.

  Renders were inspected (PNG in the scratchpad, not committed). GitHub
  renders Mermaid natively.
- The comparison with the proxy model reflects the owner's diagram. Its
  "recovery by identity_id" step is not a Kratos self-service API; this is
  stated in §10.8.
- No code changes.
