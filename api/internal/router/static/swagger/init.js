window.onload = function () {
  if (!window.SwaggerUIBundle) {
    document.body.innerHTML = '<p>Failed to load Swagger UI.</p>';
    return;
  }
  window.SwaggerUIBundle({
    url: window.location.origin + '/api/swagger/openapi.yaml',
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    layout: 'StandaloneLayout'
  });
};
