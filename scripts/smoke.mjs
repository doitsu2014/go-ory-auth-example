#!/usr/bin/env node
// End-to-end smoke test against the running local stack (unit U7).
// Zero dependencies: Node >= 20 (global fetch).
//
//   node scripts/smoke.mjs
//
// Creates throw-away identities and deletes them at the end.
import { randomUUID, randomBytes } from "node:crypto";
import { execFileSync } from "node:child_process";

const KRATOS = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433";
const KRATOS_ADMIN = process.env.KRATOS_ADMIN_URL ?? "http://127.0.0.1:4434";
const API = process.env.API_URL ?? "http://localhost:8080";
const MAILPIT = process.env.MAILPIT_URL ?? "http://localhost:8025";
const HYDRA = process.env.HYDRA_PUBLIC_URL ?? "http://localhost:4444";
const HYDRA_ADMIN = process.env.HYDRA_ADMIN_URL ?? "http://127.0.0.1:4445";
const ORIGIN = process.env.ADMIN_ORIGIN ?? "http://localhost:5173";

const created = [];
const createdClients = [];
let failures = 0;

function check(name, ok, extra = "") {
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? `  (${extra})` : ""}`);
  if (!ok) failures++;
}

async function json(res) {
  const text = await res.text();
  try {
    return text ? JSON.parse(text) : {};
  } catch {
    return { raw: text };
  }
}

const strongPassword = () => `Sm0ke-${randomBytes(12).toString("base64url")}`;

// ---------- cookie jar for browser flows ----------
class Jar {
  cookies = new Map();
  store(res) {
    for (const c of res.headers.getSetCookie?.() ?? []) {
      const [pair] = c.split(";");
      const i = pair.indexOf("=");
      this.cookies.set(pair.slice(0, i), pair.slice(i + 1));
    }
  }
  header() {
    return [...this.cookies].map(([k, v]) => `${k}=${v}`).join("; ");
  }
}

// Customer login, registration and recovery go through identity-service
// (ADR-0014): it finds the opaque Kratos handle of the address and drives the
// Kratos native flow. Kratos never receives the address itself.
// Kratos answers 500 (→ 503) when its HaveIBeenPwned lookup times out; retry.
async function customerAuth(path, body) {
  let out;
  for (let attempt = 0; attempt < 3; attempt++) {
    const res = await fetch(`${API}/v1/auth/${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    });
    out = { status: res.status, body: await json(res), text: "" };
    if (res.status < 500) break;
    await new Promise((r) => setTimeout(r, 2000));
  }
  out.text = JSON.stringify(out.body);
  return out;
}

// A well-formed handle identity-service never stored.
const randomHandle = () => `${randomBytes(32).toString("hex").replace(/[^a-z2-7]/g, "a").slice(0, 52)}@login.invalid`;

// The customer schema holds the pseudonymous login_id only; the real name is
// encrypted personal info (NAME-FR-01).
// Kratos answers 500 when its HaveIBeenPwned lookup times out; retry a few times.
async function nativeRegister(loginId, password, traits = { login_id: loginId }) {
  let out;
  for (let attempt = 0; attempt < 3; attempt++) {
    const flow = await json(await fetch(`${KRATOS}/self-service/registration/api`));
    const res = await fetch(`${KRATOS}/self-service/registration?flow=${flow.id}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ method: "password", password, traits }),
    });
    out = { status: res.status, body: await json(res) };
    if (res.status < 500) break;
    await new Promise((r) => setTimeout(r, 2000));
  }
  return out;
}

async function nativeLogin(identifier, password) {
  const flow = await json(await fetch(`${KRATOS}/self-service/login/api`));
  const res = await fetch(`${KRATOS}/self-service/login?flow=${flow.id}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ method: "password", identifier, password }),
  });
  return { status: res.status, body: await json(res) };
}

async function browserLogin(identifier, password) {
  const jar = new Jar();
  const init = await fetch(`${KRATOS}/self-service/login/browser`, { headers: { Accept: "application/json" }, redirect: "manual" });
  jar.store(init);
  const flow = await json(init);
  const csrf = flow.ui?.nodes?.find((n) => n.attributes?.name === "csrf_token")?.attributes?.value;
  const res = await fetch(`${KRATOS}/self-service/login?flow=${flow.id}`, {
    method: "POST",
    redirect: "manual",
    headers: { "Content-Type": "application/json", Accept: "application/json", Cookie: jar.header(), Origin: ORIGIN },
    body: JSON.stringify({ method: "password", identifier, password, csrf_token: csrf }),
  });
  jar.store(res);
  return { status: res.status, body: await json(res), jar };
}

async function mailCount(email) {
  const list = await json(await fetch(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`));
  return list.messages?.length ?? 0;
}

// Waits until more than `after` mails exist for `email`, then returns the
// 6-digit code from the newest one.
async function latestCodeFor(email, after = 0) {
  for (let i = 0; i < 30; i++) {
    const list = await json(await fetch(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`));
    const msg = (list.messages?.length ?? 0) > after ? list.messages[0] : undefined;
    if (msg) {
      const full = await json(await fetch(`${MAILPIT}/api/v1/message/${msg.ID}`));
      const code = String(full.Text ?? "").match(/\b(\d{6})\b/)?.[1];
      if (code) return code;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  return null;
}

// Raw row from the identity DB as text, read through the postgres container.
function rawIdentityRow(sql, db = "identity") {
  try {
    return execFileSync("docker", ["compose", "--env-file", "deploy/compose/.env", "-f", "deploy/compose/docker-compose.yml",
      "exec", "-T", "postgres", "psql", "-U", "postgres", "-d", db, "-Atc", sql], { encoding: "utf8" });
  } catch {
    return null;
  }
}

// PLI-NFR-01: no Kratos table that holds identifiers contains the address.
function kratosHolds(value) {
  const tables = ["identities", "identity_credential_identifiers", "identity_verifiable_addresses",
    "identity_recovery_addresses", "courier_messages"];
  const q = tables.map((t) => `SELECT count(*) FROM ${t} x WHERE x::text ILIKE '%' || $$${value}$$ || '%'`).join(" UNION ALL ");
  const out = rawIdentityRow(`SELECT sum(count) FROM (${q}) c`, "kratos");
  return out === null ? null : Number(out.trim());
}

// Encrypted personal information (PII-FR-01/02/04, PII-NFR-04, NAME-FR-02/03).
async function piiChecks(token, identityId) {
  const auth = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
  const phoneDigits = `9${Math.floor(10000000 + Math.random() * 89999999)}`;
  const nid = `079${Math.floor(100000000 + Math.random() * 899999999)}`;
  const first = `Khoa${randomBytes(4).toString("hex")}`;
  const last = `Smokeson${randomBytes(4).toString("hex")}`;
  const info = {
    name: { first: ` ${first} `, last },
    phone_number: `+84 ${phoneDigits}`,
    date_of_birth: "1990-05-17",
    address: { line1: "12 Smoke Street", city: "Ho Chi Minh City", country: "VN" },
    national_id: { type: "cccd", number: nid },
  };
  let res = await fetch(`${API}/v1/me/personal-info`, { method: "PUT", headers: auth, body: JSON.stringify(info) });
  let body = await json(res);
  check("PII-FR-02 PUT /v1/me/personal-info stores and normalises", res.status === 200 && body.phone_number === `+84${phoneDigits}`, `status ${res.status} ${body.code ?? ""}`);
  check("NAME-FR-02 PUT stores the name (parts trimmed)", res.status === 200 && body.name?.first === first && body.name?.last === last, `status ${res.status}`);

  res = await fetch(`${API}/v1/me/personal-info`, { headers: auth });
  body = await json(res);
  check("PII-FR-01 GET /v1/me/personal-info decrypts", res.status === 200 && body.national_id?.number === nid && body.address?.city === "Ho Chi Minh City", `status ${res.status}`);
  check("NAME-FR-02 GET /v1/me/personal-info returns the name", res.status === 200 && body.name?.first === first && body.name?.last === last, `status ${res.status}`);

  res = await fetch(`${API}/v1/me`, { headers: auth });
  body = await json(res);
  check("NAME-FR-07 GET /v1/me carries no name", res.status === 200 && !("name" in body), `status ${res.status}`);

  if (identityId) {
    const row = rawIdentityRow(`SELECT t::text FROM customer_pii t WHERE identity_id = '${identityId}'`);
    check("PII-NFR-04 raw customer_pii row holds no plaintext", row !== null && row.trim() !== "" &&
      !row.includes(phoneDigits) && !row.includes(nid) && !row.includes("Smoke Street") && !row.includes("1990-05-17"),
      row === null ? "psql unavailable" : "");
    const nameCt = rawIdentityRow(`SELECT encode(name_ct, 'hex') FROM customer_pii WHERE identity_id = '${identityId}'`)?.trim() ?? "";
    const hex = (v) => Buffer.from(v).toString("hex");
    check("NAME-FR-03 raw name_ct is ciphertext without the name", nameCt.startsWith("01") && nameCt.length > 2 * 28 &&
      ![first, last].some((v) => row?.includes(v) || nameCt.includes(hex(v))), row === null ? "psql unavailable" : `${nameCt.length / 2} bytes`);
  }

  res = await fetch(`${API}/v1/me/personal-info`, { method: "DELETE", headers: auth });
  check("PII-FR-04 DELETE /v1/me/personal-info (crypto-shred)", res.status === 204, `status ${res.status}`);
  res = await fetch(`${API}/v1/me/personal-info`, { headers: auth });
  body = await json(res);
  check("PII-FR-04 after erase all fields are null", res.status === 200 && body.phone_number === null && body.national_id === null, `status ${res.status}`);
  if (identityId) {
    const keys = rawIdentityRow(`SELECT count(*) FROM subject_key WHERE identity_id = '${identityId}'`);
    check("PII-FR-04 subject data key destroyed", keys?.trim() === "0", `count ${keys?.trim()}`);
  }
}

// Machine-to-machine via Hydra client_credentials (M2M-FR-01..05, 13).
async function m2mChecks(customerId, sessionToken) {
  let out;
  try {
    out = execFileSync("docker", ["compose", "--env-file", "deploy/compose/.env", "-f", "deploy/compose/docker-compose.yml",
      "--profile", "app", "exec", "-T", "identity-service", "/identity-service", "clients", "create",
      "--name", `smoke-${randomBytes(3).toString("hex")}`, "--owner", "smoke@example.local", "--scope", "customers:read"], { encoding: "utf8" });
  } catch {
    check("M2M-FR-13 CLI creates a service client", false, "docker exec failed");
    return;
  }
  const clientId = out.match(/client_id=(\S+)/)?.[1];
  const secret = out.match(/client_secret[^:]*: (\S+)/)?.[1];
  check("M2M-FR-13 CLI creates a service client (secret shown once)", !!clientId && !!secret);
  if (!clientId || !secret) return;
  createdClients.push(clientId);
  const basic = "Basic " + Buffer.from(`${encodeURIComponent(clientId)}:${encodeURIComponent(secret)}`).toString("base64");
  const token = async (params) => {
    const r = await fetch(`${HYDRA}/oauth2/token`, { method: "POST",
      headers: { Authorization: basic, "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams(params) });
    return [r.status, await json(r)];
  };
  const [ts, t] = await token({ grant_type: "client_credentials", scope: "customers:read", audience: "identity-service" });
  check("M2M-FR-01 Hydra issues a client_credentials JWT", ts === 200 && t.access_token?.split(".").length === 3, `status ${ts}`);
  const [bs, b] = await token({ grant_type: "client_credentials", scope: "audit:read", audience: "identity-service" });
  check("M2M-FR-01 Hydra refuses a scope the client lacks", bs === 400 && b.error === "invalid_scope", `status ${bs} ${b.error ?? ""}`);
  if (!t.access_token) return;
  const bearer = { Authorization: `Bearer ${t.access_token}` };

  let res = await fetch(`${API}/m2m/v1/customers/${customerId}`, { headers: bearer });
  let body = await json(res);
  check("M2M-FR-05 GET /m2m/v1/customers/{id} returns status without PII", res.status === 200 && body.id === customerId &&
    !("email" in body) && !("name" in body), `status ${res.status}`);
  res = await fetch(`${API}/m2m/v1/audit-events`, { headers: bearer });
  check("M2M-FR-04 missing scope is 403 insufficient_scope", res.status === 403 &&
    (res.headers.get("www-authenticate") ?? "").includes("insufficient_scope"), `status ${res.status}`);
  res = await fetch(`${API}/m2m/v1/customers/${customerId}`, { headers: { Authorization: `Bearer ${sessionToken}` } });
  check("Plane binding: Kratos session token rejected on /m2m/v1", res.status === 401, `status ${res.status}`);
  res = await fetch(`${API}/v1/me`, { headers: bearer });
  check("Plane binding: Hydra JWT rejected on /v1", res.status === 401, `status ${res.status}`);
  const [nas, na] = await token({ grant_type: "client_credentials", scope: "customers:read" });
  if (nas === 200 && na.access_token) {
    res = await fetch(`${API}/m2m/v1/customers/${customerId}`, { headers: { Authorization: `Bearer ${na.access_token}` } });
    check("M2M-FR-03 token without audience is 401", res.status === 401, `status ${res.status}`);
  } else {
    check("M2M-FR-03 token without audience is 401", false, `Hydra did not issue the no-audience token (status ${nas})`);
  }
}

async function main() {
  // ---- Customer: register (FR-01) ----
  const email = `smoke-${randomUUID()}@example.local`;
  const password = strongPassword();
  let currentPassword = password;
  const withName = await nativeRegister(null, password, { login_id: randomHandle(), name: { first: "Smoke", last: "Test" } });
  check("NAME-FR-01 native registration with traits.name is rejected (400)", withName.status === 400, `status ${withName.status}`);
  if (withName.body.identity?.id) created.push(withName.body.identity.id);
  const plain = await nativeRegister(null, password, { email: `smoke-plain-${randomUUID()}@example.local` });
  check("PLI-FR-04 registration with a plaintext email is rejected (4049001)", plain.status === 400 &&
    JSON.stringify(plain.body).includes("4049001"), `status ${plain.status}`);
  const unresolved = await nativeRegister(randomHandle(), password);
  check("PLI-FR-04 direct Kratos registration with a handle identity-service did not store is rejected (4049002)", unresolved.status === 400 &&
    JSON.stringify(unresolved.body).includes("4049002"), `status ${unresolved.status}`);
  const oldResolve = await fetch(`${API}/v1/auth/identifiers`, { method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ type: "email", value: email, purpose: "sign_in" }) });
  check("PLX-FR-05 the resolve endpoint is gone (404)", oldResolve.status === 404, `status ${oldResolve.status}`);
  const reg = await customerAuth("registration", { login: { type: "email", value: email.toUpperCase() }, password });
  const loginId = reg.body.session?.identity?.traits?.login_id;
  check("PLX-FR-02 registration through identity-service returns a session token", reg.status === 200 && !!reg.body.session_token, `status ${reg.status} ${reg.body.code ?? ""}`);
  check("PLX-FR-04 Kratos handle is opaque", /^[a-z2-7]{52}@login\.invalid$/.test(loginId ?? ""), String(loginId));
  const token = reg.body.session_token;
  const customerId = reg.body.session?.identity?.id;
  if (customerId) created.push(customerId);
  if (customerId) {
    const k = await json(await fetch(`${KRATOS_ADMIN}/admin/identities/${customerId}`));
    check("PLI-FR-03 Kratos traits hold the handle only", k.traits?.login_id === loginId && !("email" in (k.traits ?? {})) &&
      Object.keys(k.traits ?? {}).length === 1, `keys ${Object.keys(k.traits ?? {}).join(",")}`);
  }
  const dup = await customerAuth("registration", { login: { type: "email", value: email }, password });
  check("PLX-FR-02 duplicate registration is rejected (4000007)", dup.status === 400 && dup.body.code === "auth_flow_rejected" &&
    dup.text.includes("4000007"), `status ${dup.status}`);

  // ---- Customer: API with bearer (FR-08/FR-09) ----
  let res = await fetch(`${API}/v1/me`, { headers: { Authorization: `Bearer ${token}` } });
  let me = await json(res);
  check("FR-08/09 GET /v1/me with bearer token", res.status === 200 && me.email === email, `status ${res.status}`);
  check("PLI-FR-07 GET /v1/me returns the decrypted login", me.login?.type === "email" && me.login?.value === email, JSON.stringify(me.login ?? {}));

  res = await fetch(`${API}/v1/me`);
  check("FR-08 GET /v1/me without credential is 401", res.status === 401);

  res = await fetch(`${API}/admin/v1/me`, { headers: { Authorization: `Bearer ${token}` } });
  check("Plane binding: bearer rejected on /admin/v1", res.status === 401, `status ${res.status}`);

  // ---- Customer: verified email gate + verification (FR-03, FR-10) ----
  res = await fetch(`${API}/v1/me`, {
    method: "PATCH",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify({ display_name: "Smokey" }),
  });
  const p = await json(res);
  check("FR-10 PATCH /v1/me before verification is 403 email_not_verified", res.status === 403 && p.code === "email_not_verified", `status ${res.status} ${p.code ?? ""}`);

  // Native registration does not return show_verification_ui, so the client
  // starts its own native verification flow (which emails a fresh code).
  await latestCodeFor(email); // registration mail
  const vflow = await json(await fetch(`${KRATOS}/self-service/verification/api`));
  const vflowId = vflow.id;
  const before = await mailCount(email);
  await fetch(`${KRATOS}/self-service/verification?flow=${vflowId}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    // The signed-in owner addresses Kratos with their own handle (whoami).
    body: JSON.stringify({ method: "code", email: loginId }),
  });
  const code = await latestCodeFor(email, before);
  check("FR-03 / PLI-FR-05 verification code delivered to the real address", !!code && !!vflowId);
  if (code && vflowId) {
    res = await fetch(`${KRATOS}/self-service/verification?flow=${vflowId}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ method: "code", code }),
    });
    const v = await json(res);
    check("FR-03 verification code accepted", res.status === 200 && v.state === "passed_challenge", `status ${res.status} ${v.state ?? ""}`);
  }

  // The session cache may hold the pre-verification session for <= 30 s; log in again for a fresh session.
  const login = await customerAuth("login", { login: { type: "email", value: ` ${email.toUpperCase()} ` }, password });
  check("PLX-FR-01 login through identity-service with the email", login.status === 200 && !!login.body.session_token, `status ${login.status}`);
  const wrong = await customerAuth("login", { login: { type: "email", value: email }, password: "not the password at all" });
  const unknown = await customerAuth("login", { login: { type: "email", value: `smoke-nobody-${randomUUID()}@example.local` }, password });
  const shape = (r) => JSON.stringify({ s: r.status, c: r.body.code, e: r.body.errors });
  check("PLX-FR-01 wrong password and unknown address are rejected identically", wrong.status === 400 && shape(wrong) === shape(unknown) &&
    wrong.text.includes("4000006"), `${shape(wrong)} vs ${shape(unknown)}`);
  check("PLX-FR-06 rejections never carry a handle", ![wrong, unknown, dup].some((r) => r.text.includes("login.invalid")));
  if (login.body.session_token) {
    res = await fetch(`${API}/v1/me`, {
      method: "PATCH",
      headers: { Authorization: `Bearer ${login.body.session_token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ display_name: "Smokey" }),
    });
    me = await json(res);
    check("FR-10 PATCH /v1/me after verification", res.status === 200 && me.display_name === "Smokey", `status ${res.status}`);
    const customerId = reg.body.session?.identity?.id ?? login.body.session?.identity?.id;
    await piiChecks(login.body.session_token, customerId);
    if (customerId) await m2mChecks(customerId, login.body.session_token);
  }

  // ---- Customer: password recovery (FR-04, PLX-FR-03) ----
  const beforeRec = await mailCount(email);
  const rstart = await customerAuth("recovery", { login: { type: "email", value: email } });
  const rnone = await customerAuth("recovery", { login: { type: "email", value: `smoke-nobody-${randomUUID()}@example.local` } });
  check("PLX-FR-03 recovery starts through identity-service, same answer for unknown addresses",
    rstart.status === 200 && rnone.status === 200 && !!rstart.body.recovery_id && !!rnone.body.recovery_id &&
    Object.keys(rstart.body).join() === Object.keys(rnone.body).join(), `status ${rstart.status}/${rnone.status}`);
  check("PLX-FR-03 recovery never hands out a Kratos flow id", !("flow_id" in rstart.body) &&
    !/[0-9a-f]{8}-[0-9a-f]{4}-/.test(rstart.text), rstart.text.slice(0, 40));
  const forged = await customerAuth("recovery/code", { recovery_id: "vault:v1:Zm9yZ2Vk", code: "123456" });
  check("PLX-FR-03 a forged recovery_id is rejected (422)", forged.status === 422, `status ${forged.status}`);
  const rcode = await latestCodeFor(email, beforeRec);
  const wrongCode = await customerAuth("recovery/code", { recovery_id: rstart.body.recovery_id, code: rcode === "000000" ? "111111" : "000000" });
  check("PLX-FR-03 wrong recovery code is rejected (4060006)", wrongCode.status === 400 && wrongCode.text.includes("4060006"), `status ${wrongCode.status}`);
  const grant = await customerAuth("recovery/code", { recovery_id: rstart.body.recovery_id, code: rcode });
  const recToken = grant.body.session_token;
  const settingsId = grant.body.settings_flow_id;
  check("FR-04 recovery code returns a privileged session token + settings flow", grant.status === 200 && !!recToken && !!settingsId, `status ${grant.status} ${grant.body.code ?? ""}`);
  const newPassword = strongPassword();
  if (recToken && settingsId) {
    res = await fetch(`${KRATOS}/self-service/settings?flow=${settingsId}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", "X-Session-Token": recToken },
      body: JSON.stringify({ method: "password", password: newPassword }),
    });
    check("FR-04 new password set via settings flow", res.status === 200, `status ${res.status}`);
    const relogin = await customerAuth("login", { login: { type: "email", value: email }, password: newPassword });
    check("FR-04 login with the new password", relogin.status === 200, `status ${relogin.status}`);
    if (relogin.status === 200) currentPassword = newPassword;
  }

  // ---- Population guard (ADR-0004) ----
  const custBrowser = await browserLogin(loginId, currentPassword);

  // ---- Phone login through the SMS sink (PLI-FR-05/08) ----
  const phone = `+849${Math.floor(10000000 + Math.random() * 89999999)}`;
  const preg = await customerAuth("registration", { login: { type: "phone", value: phone.replace("+84", "0") }, password });
  if (preg.body.session?.identity?.id) created.push(preg.body.session.identity.id);
  const sink = `${phone.slice(1)}@sms.local`;
  const pcode = await latestCodeFor(sink);
  check("PLI-FR-05 phone registration: code delivered as SMS (local sink)", preg.status === 200 && !!pcode, `status ${preg.status}`);
  const pme = await json(await fetch(`${API}/v1/me`, { headers: { Authorization: `Bearer ${preg.body.session_token}` } }));
  check("PLI-FR-07 phone customer: /v1/me login is the phone, no email", pme.login?.type === "phone" && pme.login?.value === phone && !("email" in pme), JSON.stringify(pme.login ?? {}));

  // ---- No customer address in the Kratos database (PLI-NFR-01) ----
  for (const [label, v] of [["email", email], ["phone", phone.slice(3)]]) {
    const n = kratosHolds(v);
    check(`PLI-NFR-01 Kratos tables hold no customer ${label}`, n === 0, n === null ? "psql unavailable" : `${n} rows`);
  }
  check("Guard: customer cannot log in via browser flow", custBrowser.status >= 400, `status ${custBrowser.status}`);

  const adminEmail = `smoke-admin-${randomUUID()}@example.local`;
  const adminPassword = strongPassword();
  res = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      schema_id: "admin",
      traits: { email: adminEmail },
      credentials: { password: { config: { password: adminPassword } } },
    }),
  });
  const admin = await json(res);
  check("Admin identity created via Kratos admin API", res.status === 201, `status ${res.status}`);
  if (admin.id) created.push(admin.id);

  const adminNative = await nativeLogin(adminEmail, adminPassword);
  check("Guard: admin cannot log in via native API flow", adminNative.status >= 400, `status ${adminNative.status}`);

  // ---- Admin plane (FR-05/FR-06) ----
  const adminBrowser = await browserLogin(adminEmail, adminPassword);
  check("FR-05 admin browser login (aal1)", adminBrowser.status === 200, `status ${adminBrowser.status}`);
  res = await fetch(`${API}/admin/v1/me`, { headers: { Cookie: adminBrowser.jar.header(), Origin: ORIGIN } });
  const am = await json(res);
  check("FR-06 admin without TOTP gets 403 mfa_enrollment_required", res.status === 403 && am.code === "mfa_enrollment_required", `status ${res.status} ${am.code ?? ""}`);

  res = await fetch(`${API}/admin/v1/customers`, { headers: { Cookie: adminBrowser.jar.header(), Origin: ORIGIN } });
  check("FR-06 admin at AAL1 cannot list customers", res.status === 403, `status ${res.status}`);

  res = await fetch(`${API}/v1/me`, { headers: { Cookie: adminBrowser.jar.header() } });
  check("Plane binding: cookie rejected on /v1", res.status === 401, `status ${res.status}`);
}

try {
  await main();
} catch (e) {
  console.error("ERROR", e);
  failures++;
} finally {
  for (const id of createdClients) {
    await fetch(`${HYDRA_ADMIN}/admin/clients/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(() => {});
  }
  for (const id of created) {
    await fetch(`${KRATOS_ADMIN}/admin/identities/${id}`, { method: "DELETE" }).catch(() => {});
  }
  console.log(failures ? `\n${failures} check(s) failed` : "\nAll smoke checks passed");
  process.exit(failures ? 1 : 0);
}
