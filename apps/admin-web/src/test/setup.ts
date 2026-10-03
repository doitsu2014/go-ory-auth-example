import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";

import i18n from "../i18n";
import { clearToasts } from "../shared/toast";
import { server } from "./server";

// jsdom's AbortSignal is not an instance of Node's (undici) AbortSignal, and
// React Router passes one to `new Request()`. Drop jsdom signals in tests.
const NodeRequest = globalThis.Request;
class TestRequest extends NodeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(input, init?.signal ? { ...init, signal: null } : init);
  }
}
globalThis.Request = TestRequest;

beforeAll(async () => {
  server.listen({ onUnhandledRequest: "error" });
  // Tests assert English UI strings; vi (the default) is covered explicitly.
  await i18n.changeLanguage("en");
});

afterEach(async () => {
  cleanup();
  server.resetHandlers();
  clearToasts();
  await i18n.changeLanguage("en");
});

afterAll(() => server.close());
