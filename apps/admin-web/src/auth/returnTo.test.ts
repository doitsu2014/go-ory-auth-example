import { describe, expect, it } from "vitest";

import { loginPath, safeReturnTo } from "./returnTo";

describe("safeReturnTo", () => {
  it.each([
    ["/customers?state=active", "/customers?state=active"],
    [`${window.location.origin}/audit`, "/audit"],
  ])("accepts %s", (input, expected) => {
    expect(safeReturnTo(input)).toBe(expected);
  });

  it.each([
    "https://evil.example.com/x",
    "//evil.example.com/x",
    "/\\evil.example.com",
    "javascript:alert(1)",
    "customers",
    "/login?return_to=/x",
    "",
    null,
  ])("rejects %s", (input) => {
    expect(safeReturnTo(input)).toBeUndefined();
  });

  it("builds login paths", () => {
    expect(loginPath({ aal2: true, returnTo: "/admins" })).toBe(
      "/login?aal=aal2&return_to=%2Fadmins",
    );
    expect(loginPath({ returnTo: "https://evil.example.com" })).toBe("/login");
  });
});
