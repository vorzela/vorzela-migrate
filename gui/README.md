# Vorzela Migrate GUI (VorzelaJS)

Client-side VorzelaJS app for `vm gui`. It uses the file router and Vorzela UI
components copied from the registry.

See [`../GUI.md`](../GUI.md) for the user-facing workflow and security model.

## Develop

The `@vorzelajs/*` dependencies currently point at the sibling
`../../vorzelajs/packages/*` checkout used by this workspace. The checked-in
build in `../internal/gui/web/dist` means people who only install `vm` do not
need Node. If you rebuild the GUI elsewhere, replace those `file:` entries with
the published `@vorzelajs` versions (and configure your registry token), or copy
the sibling packages next to this repository.

```bash
npm install
npm run dev
```

The Vite dev server runs on `http://localhost:5173`. Start the API separately:

```bash
# from the repository root
go build -o vm ./cmd/vm
./vm gui --no-open --dev-origin http://localhost:5173
```

Vite proxies `/api` to `http://127.0.0.1:7845` (see `vite.config.ts`).

## Build and embed

```bash
npm run build
# from the repository root
rm -rf internal/gui/web/dist
cp -R gui/dist/client internal/gui/web/dist
go build -o vm ./cmd/vm
```

Or run `make gui` from the repository root.

## UI components

The component and style source is copied into the app shadcn-style:

```bash
node ../../vorzela-ui/vorzelajs/packages/cli/bin/vorzelajs-ui.mjs add button
```

`vorzelajs.ui.json` records aliases and paths. Generated files carry a
`@generated` header; put style overrides in `src/styles/custom/`.

This app carries a few small patches to installed components:

- `badge.tsx` / `badge.module.scss`: added a `warning` variant.
- `sidebar.tsx`: the icon slot renders DOM nodes directly (`NavIcon`) instead
  of stringifying a function child.
- `app-shell.tsx` / `topbar.tsx`: `title` accepts `ReactNode` so the topbar can
  follow route changes with a reactive text accessor.

Re-apply these if you run `vorzelajs-ui update` with `--force`.

## Checks

```bash
npm run typecheck
npm run lint
npm run build
```
