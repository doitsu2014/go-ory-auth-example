# Intent capture questions

The workflow ran in YOLO mode, so no questions file was presented. The one
open decision was asked in chat before the workflow started.

1. Which direction should the refactor take?
   - A. Proxy + opaque id (recommended)
   - B. Proxy, keeping the HMAC pseudonym in Kratos
   - C. Keep the current model and only fix the docs

   [Answer]: A (product owner, 2026-10-04)

Recommendations recorded without asking:

- Verification and password change stay direct to Kratos. They use the
  session's own handle, so no oracle is involved.
- Recovery start goes through the proxy using the stock Kratos code flow, not
  the admin recovery-code API from the diagram. The response is then the same
  for known and unknown addresses, and the code and new password still go
  straight to Kratos.
- Kratos errors come back as problem `auth_flow_rejected` with Kratos message
  ids only, never the flow. The flow echoes the identifier, and echoing it
  would recreate the oracle.
