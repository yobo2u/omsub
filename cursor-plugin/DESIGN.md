# Cursor Plugin Management UI

## 1. Product context

The page is an operational surface served directly by the Cursor plugin. It must remain usable without CPA Manager Plus or CLIProxyAPI frontend source changes. The primary users are self-hosting operators who need to inspect Cursor account health and prevent selected models from being advertised or executed.

## 2. Direction

Preserve the compact, neutral system-tool character of CLIProxyAPI's own plugin resource examples. The memorable element is the explicit separation between authoritative subscription quota and local estimated usage: unknown data is shown honestly, never converted into a misleading progress bar.

## 3. Tokens

- Typeface: system UI for content; system monospace for model IDs and metrics.
- Canvas: `--canvas`; raised panel: `--surface`; subtle panel: `--surface-muted`.
- Text: `--text`; secondary text: `--muted`; border: `--border`.
- Accent: `--accent`; success: `--success`; warning: `--warning`; danger: `--danger`.
- Spacing follows a 4 px base unit: 4, 8, 12, 16, 24, 32.
- Radius: 8 px controls, 12 px cards. Shadow is limited to the account card.

## 4. Layout and responsiveness

- A centered shell with a readable maximum width and document scrolling.
- Header and credential panel appear first; account cards follow.
- Account summary becomes a single column below 720 px.
- Model choices use a responsive grid and never force horizontal scrolling.

## 5. Primitives and states

- `Panel`: default and warning variants.
- `Field`: label, password input, helper text, focus-visible state.
- `Button`: primary, secondary, loading, disabled, focus-visible states.
- `Metric`: label and monospace value.
- `ModelOption`: checked, unchecked, keyboard-focus, disabled states.
- `Notice`: information, success, and error states with `role=status` or `role=alert`.
- `LanguageSelect`: native select with Chinese and English options, keyboard focus, and persisted preference.

## 6. Interaction

- The management key remains in page memory only and is never persisted.
- “加载状态” fetches the authenticated plugin status endpoint.
- “全部禁用 / Disable all” selects every available model for the current account but does not persist until the explicit save action.
- “保存模型禁用” submits only the selected account and model IDs.
- Buttons expose loading and disabled states; completion is announced to assistive technology.
- The initial language follows a previously saved preference, then the browser language, with English as the fallback.
- Changing language immediately updates static copy, dynamic account cards, notices, the document title, and the document language. Only the language preference is persisted.
- No decorative animation. Reduced-motion users receive the same instantaneous state changes.

## 7. Accessibility

- Semantic headings, labels, fieldsets, legends, and buttons.
- Visible keyboard focus and minimum 44 px interactive height.
- Status is not conveyed by color alone.
- The language selector uses a native labelled control, and `<html lang>` always matches the displayed language.
- Chinese copy uses natural line breaks; model IDs may break only at safe punctuation.
- English copy uses concise operational language and remains readable without horizontal scrolling.

## 8. Accepted debt

- Cursor does not publish a stable OAuth subscription remaining-quota API. The page therefore displays an unavailable state and local estimated usage instead of a fabricated remainder.
- The standalone page asks for the management key because plugin browser resources are intentionally unauthenticated by the host and cannot safely mutate state by themselves.

## Reference

Visual grammar is extracted from CLIProxyAPI's `examples/plugin/host-callback-auth-files` resource page: system typography, restrained neutral surfaces, direct operational copy, and no external assets.
