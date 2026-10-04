// Kratos http courier (ADR-0013, A4): only the recipient, the template
// type, the identity id and the one-time code leave Kratos. Kratos-rendered
// subjects, bodies and URLs are never sent; identity-service renders its own
// messages and resolves pseudonym recipients to the real address.
local field(o, k) = if std.objectHas(o, k) then o[k] else null;
function(ctx) {
  local data = ctx.template_data,
  recipient: ctx.recipient,
  template_type: ctx.template_type,
  identity_id: if std.objectHas(data, 'identity') then data.identity.id else null,
  code: std.foldl(function(acc, k) if acc != null then acc else field(data, k), ['verification_code', 'recovery_code', 'login_code', 'registration_code'], null),
  expires_in_minutes: field(data, 'expires_in_minutes'),
}
