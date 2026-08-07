import js from "@eslint/js";
import globals from "globals";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

// Flat-config ESLint for the frontend.
//
// - Application/library sources under src/ are linted with full type-aware
//   rules (typescript-eslint's recommendedTypeChecked), plus the classic
//   react-hooks rules (rules-of-hooks + exhaustive-deps) and react-refresh.
//   The newer React-Compiler rules bundled in react-hooks v7's presets are
//   deliberately left off: several fight intentional engine-sync effects here
//   (the mount-only setPattern seed, the parameter-push effects), and F4 asks
//   for the hook-rule "basics", not the compiler lint.
// - Config files and the Playwright e2e specs run Node and live outside the
//   app tsconfig, so they get the non-type-checked recommended set with Node
//   globals — type-aware parsing there would need them in a tsconfig project.
export default tseslint.config(
  {
    ignores: [
      "dist/**",
      "node_modules/**",
      // Generated / third-party runtime assets served as-is from public/.
      "public/**",
      "playwright-report/**",
      "test-results/**",
    ],
  },
  {
    files: ["src/**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
    ],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser, ...globals.worker },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      "jsx-a11y": jsxA11y,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...jsxA11y.flatConfigs.recommended.rules,
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
    },
  },
  {
    files: ["e2e/**/*.ts", "*.config.{ts,js}", "eslint.config.js"],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.node },
    },
  },
);
