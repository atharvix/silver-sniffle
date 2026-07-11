import { pgTable, text, integer, timestamp } from "drizzle-orm/pg-core";

// Opaque bearer tokens issued after a successful OTP verification. Profile
// endpoints resolve "Authorization: Bearer <token>" to an email via this
// table. Persisted (rather than an in-memory Map) so a server restart or
// deploy does not invalidate every signed-in user's session at once.
export const verificationTokensTable = pgTable("verification_tokens", {
  token: text("token").primaryKey(),
  email: text("email").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).defaultNow().notNull(),
});

// One-time codes sent to an email during sign-up/sign-in. Persisted so a
// restart mid-flow doesn't silently drop an in-flight code (the user would
// otherwise get a confusing "No OTP found" error even though they just
// requested one).
export const otpCodesTable = pgTable("otp_codes", {
  email: text("email").primaryKey(),
  otp: text("otp").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  attempts: integer("attempts").notNull().default(0),
});

// Marks an email as "recently completed OTP verification", independent of
// any particular bearer token. Used only to gate /auth/send-welcome (one
// welcome email per OTP flow) and consumed (deleted) once that email is
// sent — this must stay separate from verificationTokensTable so that
// sending the welcome email does not invalidate the caller's active
// session token.
export const verifiedEmailsTable = pgTable("verified_emails", {
  email: text("email").primaryKey(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
});
