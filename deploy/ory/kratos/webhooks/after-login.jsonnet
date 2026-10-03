// Population guard (ADR-0004): identity-service rejects admins on API flows
// and customers on browser flows.
function(ctx) {
  identity_id: ctx.identity.id,
  schema_id: ctx.identity.schema_id,
  flow_type: ctx.flow.type,
}
