# Requirements questions

YOLO mode: no questions were asked. Recommended answers recorded:

1. Status of a rejected sign-in: **400 `auth_flow_rejected`**, not 401. On the
   public plane, 401 means "no session" to the app.
2. Per-account limit: **failed attempts only, 10/15m and 50/24h**. This keeps
   a victim's lockout window short while still slowing guessing.
3. Session in the response: **Kratos session JSON passed through** to the
   authenticated owner, which saves the app a `whoami` call.
