window.ui = SwaggerUIBundle({
  url: "/openapi.yaml",
  dom_id: "#swagger-ui",
  deepLinking: true,
  persistAuthorization: true,
  displayRequestDuration: true,
  tryItOutEnabled: true,
  presets: [SwaggerUIBundle.presets.apis],
  layout: "BaseLayout",
  requestInterceptor: function (req) {
    if (!req.headers["st-auth-mode"]) {
      req.headers["st-auth-mode"] = "header";
    }
    return req;
  },
});
