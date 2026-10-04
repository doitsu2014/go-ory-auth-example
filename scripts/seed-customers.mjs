#!/usr/bin/env node
// Seeds demo customers through the real flows, so their personal information
// is encrypted exactly as for a real user:
//   POST /v1/auth/registration (identity-service drives the Kratos native
//   flow) -> verification with the session's own handle (code from Mailpit;
//   SMS to the local sink <digits>@sms.local) -> POST /v1/auth/login ->
//   PATCH /v1/me (nickname) -> PUT /v1/me/personal-info
//
// Kratos never sees the email or phone number (ADR-0013, ADR-0014): it
// stores only an opaque handle. The real name is personal information, stored encrypted through
// PUT /v1/me/personal-info; display_name is a nickname, not the name.
//
//   node scripts/seed-customers.mjs        (or: ./dev seed-customers)
//
// Local stack only. Customers that already exist are skipped. All seeded
// customers share one password, printed at the end (override with
// SEED_CUSTOMER_PASSWORD). Zero dependencies: Node >= 22.
import { randomBytes } from "node:crypto";

const KRATOS = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433";
const API = process.env.API_URL ?? "http://localhost:8080";
const MAILPIT = process.env.MAILPIT_URL ?? "http://localhost:8025";
const PASSWORD = process.env.SEED_CUSTOMER_PASSWORD ?? `Seed-${randomBytes(9).toString("base64url")}`;

// Fictional people and numbers (national ids use the 0790000000xx range).
// The last customer signs in with a phone number (SMS through the local sink).
const CUSTOMERS = [
  { first: "An", last: "Nguyễn Văn", phone: "+84 901 234 501", dob: "1990-05-17", city: "Ho Chi Minh City", line1: "12 Lê Lợi", nid: "079000000001" },
  { first: "Bình", last: "Trần Thị", phone: "+84 912 345 602", dob: "1985-11-02", city: "Hà Nội", line1: "45 Hàng Bài", nid: "001000000002" },
  { first: "Cường", last: "Lê Minh", phone: "+84 933 456 703", dob: "1998-01-23", city: "Đà Nẵng", line1: "7 Bạch Đằng", nid: "048000000003" },
  { first: "Dung", last: "Phạm Thu", phone: "+84 944 567 804", dob: "1979-08-30", city: "Cần Thơ", line1: "88 Hòa Bình", nid: "092000000004" },
  { first: "Em", last: "Hoàng Gia", phone: "+84 975 678 905", dob: "2001-03-14", city: "Hải Phòng", line1: "3 Lạch Tray", nid: "031000000005" },
];

async function json(res) {
  const text = await res.text();
  try {
    return text ? JSON.parse(text) : {};
  } catch {
    return { raw: text };
  }
}

const JSON_HEADERS = { "Content-Type": "application/json", Accept: "application/json" };

// Customer registration and login through identity-service (ADR-0014).
async function customerAuth(path, login) {
  const res = await fetch(`${API}/v1/auth/${path}`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ login, password: PASSWORD }) });
  return { status: res.status, body: await json(res) };
}

// Where the code arrives in Mailpit: the email itself, or the SMS sink.
const inbox = (login) => (login.type === "phone" ? `${login.value.replace(/\D/g, "")}@sms.local` : login.value);

async function mails(email) {
  const list = await json(await fetch(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`));
  return list.messages ?? [];
}

// Waits for a verification mail created after `since` and returns its 6-digit
// code. Older mails (registration, earlier runs) are ignored.
async function codeFor(email, since) {
  for (let i = 0; i < 40; i++) {
    for (const m of await mails(email)) {
      if (Date.parse(m.Created) < since) continue;
      const full = await json(await fetch(`${MAILPIT}/api/v1/message/${m.ID}`));
      if (!/verif|xác minh|xac minh/i.test(`${m.Subject ?? ""} ${full.Text ?? ""}`)) continue;
      const code = String(full.Text ?? "").match(/\b(\d{6})\b/)?.[1];
      if (code) return code;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`no verification code for ${email}`);
}

// The signed-in owner addresses Kratos with their own handle (whoami).
async function verifyLogin(login, token) {
  // Let the registration message land first so it is not mistaken for ours.
  await new Promise((r) => setTimeout(r, 1500));
  const whoami = await json(await fetch(`${KRATOS}/sessions/whoami`, { headers: { "X-Session-Token": token, Accept: "application/json" } }));
  const email = whoami.identity?.traits?.login_id; // the handle
  const flow = await json(await fetch(`${KRATOS}/self-service/verification/api`));
  const since = Date.now() - 1000;
  await fetch(`${KRATOS}/self-service/verification?flow=${flow.id}`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ method: "code", email }) });
  const code = await codeFor(inbox(login), since);
  const res = await fetch(`${KRATOS}/self-service/verification?flow=${flow.id}`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ method: "code", code }) });
  const v = await json(res);
  if (v.state !== "passed_challenge") throw new Error(`verification failed for customer: ${res.status} ${JSON.stringify(v.ui?.messages ?? v.error ?? {})}`);
}

async function call(method, path, token, body) {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${(await json(res)).code ?? ""}`);
  return json(res);
}

async function seedOne(c, i) {
  const n = String(i + 1).padStart(2, "0");
  const login = i === CUSTOMERS.length - 1
    ? { type: "phone", value: `+849000000${n}` }
    : { type: "email", value: `customer${n}@example.local` };
  const email = login.value; // printed label only; Kratos never receives it
  // Kratos answers 500 (503 here) when its HaveIBeenPwned lookup times out; retry a few times.
  let reg;
  for (let attempt = 0; attempt < 3; attempt++) {
    reg = await customerAuth("registration", login);
    if (reg.status < 500) break;
    await new Promise((r) => setTimeout(r, 2000));
  }
  if (reg.status !== 200) {
    const exists = JSON.stringify(reg.body).includes("4000007"); // "An account with the same identifier exists already"
    console.log(`${exists ? "skip" : "FAIL"}  ${email}  ${exists ? "(already exists)" : `registration ${reg.status}`}`);
    return exists ? "skipped" : "failed";
  }
  // A late registration mail can still be mistaken for ours; a new flow fixes it.
  for (let attempt = 1; ; attempt++) {
    try {
      await verifyLogin(login, reg.body.session_token);
      break;
    } catch (err) {
      if (attempt === 3) throw err;
    }
  }
  // Fresh login: the service may cache the pre-verification session for up to 30 s.
  const signIn = await customerAuth("login", login);
  const token = signIn.body.session_token;
  if (!token) throw new Error(`login failed for ${email}: ${signIn.status}`);
  const nickname = `Khách hàng ${String(i + 1).padStart(2, "0")}`;
  await call("PATCH", "/v1/me", token, { display_name: nickname });
  await call("PUT", "/v1/me/personal-info", token, {
    name: { first: c.first, last: c.last },
    phone_number: c.phone,
    date_of_birth: c.dob,
    address: { line1: c.line1, city: c.city, country: "VN" },
    national_id: { type: "cccd", number: c.nid },
  });
  console.log(`ok    ${email}  ${nickname}`); // personal info is never printed
  return "created";
}

let created = 0;
let failed = 0;
for (const [i, c] of CUSTOMERS.entries()) {
  try {
    const r = await seedOne(c, i);
    if (r === "created") created++;
    if (r === "failed") failed++;
  } catch (err) {
    failed++;
    console.log(`FAIL  ${err.message}`);
  }
}
console.log(`\n${created} created, ${failed} failed.`);
if (created > 0) console.log(`Password for the customers created in this run (local only): ${PASSWORD}`);
process.exit(failed ? 1 : 0);
