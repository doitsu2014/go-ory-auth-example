import { describe, expect, it } from "vitest";

import { resolveRedirect } from "./useFollowRedirect";
import { kratosLoginInitAsSpaPath } from "./useKratosFlow";

describe("resolveRedirect", () => {
  it("refuses foreign URLs (open redirect)", () => {
    expect(resolveRedirect("https://evil.example.com/settings?flow=1")).toEqual({
      kind: "refused",
    });
    expect(resolveRedirect("//evil.example.com/x")).toEqual({ kind: "refused" });
    expect(resolveRedirect("javascript:alert(1)")).toEqual({ kind: "refused" });
  });

  it("keeps same-origin targets in the SPA", () => {
    expect(resolveRedirect(`${window.location.origin}/settings?flow=abc`)).toEqual({
      kind: "spa",
      path: "/settings?flow=abc",
    });
  });

  it("allows full navigation only to the Kratos public origin", () => {
    expect(resolveRedirect("http://kratos.test/self-service/recovery?code=1")).toEqual({
      kind: "external",
      url: "http://kratos.test/self-service/recovery?code=1",
    });
  });

  it("maps a Kratos login-init redirect to the SPA login route", () => {
    expect(
      kratosLoginInitAsSpaPath(
        `http://kratos.test/self-service/login/browser?aal=aal2&return_to=${encodeURIComponent(`${window.location.origin}/audit`)}`,
      ),
    ).toBe("/login?aal=aal2&return_to=%2Faudit");
    expect(
      kratosLoginInitAsSpaPath("http://kratos.test/self-service/settings/browser"),
    ).toBeUndefined();
    expect(
      kratosLoginInitAsSpaPath("https://evil.example.com/self-service/login/browser"),
    ).toBeUndefined();
  });
});
