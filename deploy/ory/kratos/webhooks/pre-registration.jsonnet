// Pre-persist registration check (ADR-0013/0014, response.parse: true): the
// login_id must be a handle identity-service stored (POST
// /v1/auth/registration); a legacy
// email trait is rejected. Only these fields leave Kratos.
function(ctx) {
  schema_id: ctx.identity.schema_id,
  flow_type: ctx.flow.type,
  login_id: if std.objectHas(ctx.identity.traits, 'login_id') then ctx.identity.traits.login_id else null,
  email: if std.objectHas(ctx.identity.traits, 'email') then ctx.identity.traits.email else null,
}
