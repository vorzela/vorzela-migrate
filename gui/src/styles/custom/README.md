# Custom styles

The CLI writes generated component styles into `components/ui` and
`components/primitives` inside this styles folder. Do not edit those files:
`vorzelajs-ui update` replaces them.

Put overrides and your own styles in this `custom` folder. `global.scss`
imports `custom/_index.scss`, so selectors added there are part of the app.

1. Add rules to `_index.scss`, or create a sibling file and import it:
   ```scss
   @use 'buttons';
   ```
2. Start every new file with:
   ```scss
   @use '..' as vz;
   @use 'sass:math';
   ```
3. Use the `vz-scss` VS Code snippet to insert the header and API notes.

Prefer semantic tokens (`:root` and `.dark` custom properties) for global
changes, and component-specific selectors or CSS variables for local changes.
