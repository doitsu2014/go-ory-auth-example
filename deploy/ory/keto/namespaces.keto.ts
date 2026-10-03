import { Namespace, Context } from "@ory/keto-namespace-types"

class User implements Namespace {}

// A single object, Console:main, represents the admin plane (ADR-0005).
class Console implements Namespace {
  related: {
    super_admins: User[]
    admins: User[]
    supporters: User[]
  }

  permits = {
    manage_admins: (ctx: Context): boolean =>
      this.related.super_admins.includes(ctx.subject),

    manage_customers: (ctx: Context): boolean =>
      this.related.super_admins.includes(ctx.subject) ||
      this.related.admins.includes(ctx.subject),

    view_customers: (ctx: Context): boolean =>
      this.permits.manage_customers(ctx) ||
      this.related.supporters.includes(ctx.subject),

    view_audit: (ctx: Context): boolean => this.permits.manage_customers(ctx),

    // Full (unmasked) customer PII; every use is audited.
    reveal_customer_pii: (ctx: Context): boolean => this.permits.manage_customers(ctx),

    // Register, rotate and delete machine-to-machine OAuth2 clients (Hydra).
    manage_service_clients: (ctx: Context): boolean =>
      this.related.super_admins.includes(ctx.subject),
  }
}
