# web/static

Offline frontend dependencies served locally (no CDN):

- `vue.global.js` — Vue 3 (full build with template compiler);
- `vue.global.prod.js` — Vue 3 (runtime-only production build, kept for reference);
- `echarts.min.js` — Apache ECharts.

Note: the directory is named `static/` (not `vendor/`) to avoid colliding
with Go's `vendor/` module convention.
