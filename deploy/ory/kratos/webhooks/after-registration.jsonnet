// Profile provisioning (ADR-0008) and login binding (ADR-0013).
function(ctx) {
  identity_id: ctx.identity.id,
  schema_id: ctx.identity.schema_id,
  flow_type: ctx.flow.type,
  [if std.objectHas(ctx.identity.traits, 'login_id') then 'login_id']: ctx.identity.traits.login_id,
}
