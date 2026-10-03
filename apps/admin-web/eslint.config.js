import js from "@eslint/js";
import prettier from "eslint-config-prettier";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: [
      "dist",
      "coverage",
      "node_modules",
      "src/api/schema.d.ts",
      "test-results",
      "playwright-report",
    ],
  },
  {
    extends: [
      js.configs.recommended,
      ...tseslint.configs.strictTypeChecked,
      jsxA11y.flatConfigs.strict,
    ],
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/restrict-template-expressions": ["error", { allowNumber: true }],
      "@typescript-eslint/no-confusing-void-expression": "off",
      "@typescript-eslint/no-misused-promises": [
        "error",
        { checksVoidReturn: { attributes: false } },
      ],
      "@typescript-eslint/only-throw-error": "off",
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: "dangerouslySetInnerHTML is forbidden (CSP / XSS policy).",
        },
        {
          selector: "MemberExpression[object.name=/^(localStorage|sessionStorage)$/]",
          message: "Do not use web storage for auth state; the session is an HttpOnly cookie.",
        },
      ],
    },
  },
  {
    files: ["src/**/*.test.{ts,tsx}", "src/test/**", "e2e/**"],
    rules: {
      "react-refresh/only-export-components": "off",
      "@typescript-eslint/no-non-null-assertion": "off",
    },
  },
  {
    files: ["*.config.{js,ts}", "e2e/**"],
    languageOptions: { globals: { ...globals.node } },
  },
  prettier,
);
