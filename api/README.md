# API description

`openapi.yaml` is the contract for every route the API serves, including the SuperTokens routes under `/auth`. It is embedded in the binary and served at `/openapi.yaml`, with Swagger UI at `/docs/` when `API_DOCS_ENABLED` is on (by default everywhere but production).

Keep it in the same PR as the route change:

- `TestOpenAPIMatchesRoutes` fails if a route is served but not described here, or described but not served.
- `TestOpenAPIRefsResolve` fails on any `$ref` that points nowhere.

Errors use the shared `Error` schema (`message`, `request_id`). Locally, open [localhost:8080/docs/](http://localhost:8080/docs/) to try the API, including signing in with a code printed in the console.
