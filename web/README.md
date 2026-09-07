# Web client boundary

The web client is deliberately deferred. A future Vue or React project should build into `web/dist` and expose a standard `build` script. The root Docker build detects npm, pnpm, or Yarn lockfiles, builds this directory, and copies only `dist/` into the runtime image. The Go service provides static assets and SPA route fallback; do not add Nginx, Caddy, or another web server to the container.

Until a frontend package is added, Docker publishes `placeholder.html` as `index.html` so the root path still has a deterministic response.
