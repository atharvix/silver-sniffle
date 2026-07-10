import { Router, type IRouter } from "express";
import { randomInt, randomBytes } from "crypto";
import { SendOtpBody, VerifyOtpBody, SendWelcomeBody } from "@workspace/api-zod";

const router: IRouter = Router();

interface OtpEntry {
  otp: string;
  expiresAt: number;
  attempts: number;
}

interface RateBucket {
  count: number;
  windowStart: number;
}

// In-memory stores
const otpStore        = new Map<string, OtpEntry>();
const sendRateByEmail = new Map<string, RateBucket>();
const sendRateByIp    = new Map<string, RateBucket>();

const EMAIL_RE            = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const OTP_TTL_MS          = 10 * 60 * 1000;
const MAX_VERIFY_ATTEMPTS = 5;
const EMAIL_WINDOW_MS     = 10 * 60 * 1000;
const EMAIL_MAX_SENDS     = 3;
const IP_WINDOW_MS        = 60 * 1000;
const IP_MAX_SENDS        = 10;

function generateOtp(): string {
  return String(randomInt(1000, 10000));
}

function isRateLimited(
  store: Map<string, RateBucket>,
  key: string,
  windowMs: number,
  maxCount: number,
): boolean {
  const now    = Date.now();
  const bucket = store.get(key);
  if (!bucket || now - bucket.windowStart > windowMs) {
    store.set(key, { count: 1, windowStart: now });
    return false;
  }
  if (bucket.count >= maxCount) return true;
  bucket.count += 1;
  return false;
}

// ── HTML escaping ─────────────────────────────────────────────────────────────

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

// ── Verification token store ───────────────────────────────────────────────────
// After a successful OTP verification, we issue a random opaque token that the
// client must present (as "Authorization: Bearer <token>") on profile endpoints.
// The token is bound server-side to the verified email address.

interface VerificationEntry {
  email: string;
  expiresAt: number;
}

const verificationTokens = new Map<string, VerificationEntry>(); // token → entry
const VERIFIED_TTL_MS = 30 * 60 * 1000; // 30 minutes

// Kept for /auth/send-welcome backward-compat: derive email presence from tokens.
// We use a separate email-keyed set so send-welcome still works without changes.
export const verifiedEmails = new Map<string, number>(); // email → expiry timestamp

/**
 * Look up and validate a verification token.
 * Returns the bound email address if the token is valid, or null if not.
 * Exported for use by profile routes.
 */
export function getEmailFromToken(token: string): string | null {
  const entry = verificationTokens.get(token);
  if (!entry) return null;
  if (Date.now() > entry.expiresAt) {
    verificationTokens.delete(token);
    return null;
  }
  return entry.email;
}

// ── Welcome rate limiting ──────────────────────────────────────────────────────
const welcomeRateByEmail = new Map<string, RateBucket>();
const welcomeRateByIp    = new Map<string, RateBucket>();
const WELCOME_EMAIL_WINDOW_MS = 60 * 60 * 1000; // 1 hour
const WELCOME_EMAIL_MAX       = 2;               // max 2 welcome emails per hour per address
const WELCOME_IP_WINDOW_MS    = 60 * 1000;       // 1 minute
const WELCOME_IP_MAX          = 20;              // max 20 per IP per minute

// ── Brevo email delivery ──────────────────────────────────────────────────────

const BREVO_TIMEOUT_MS = 10_000; // 10 s

function isBrevoConfigured(): boolean {
  return !!(process.env.BREVO_API_KEY && process.env.BREVO_SENDER_EMAIL);
}

async function sendOtpEmail(to: string, otp: string): Promise<void> {
  const apiKey      = process.env.BREVO_API_KEY!;
  const senderEmail = process.env.BREVO_SENDER_EMAIL!;

  const html = `
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#0f0a07;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#0f0a07;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%" style="max-width:480px;background:#1a0a06;border:1px solid rgba(255,255,255,0.1);border-radius:16px;padding:40px 36px;">
        <tr><td>
          <p style="margin:0 0 8px;font-size:22px;font-weight:800;color:#ffffff;letter-spacing:-0.4px;">
            Your Series code
          </p>
          <p style="margin:0 0 28px;font-size:14px;color:rgba(255,255,255,0.5);line-height:1.5;">
            Use this code to verify your email address. It expires in 10 minutes.
          </p>
          <div style="background:rgba(255,255,255,0.07);border:1px solid rgba(255,255,255,0.15);border-radius:12px;padding:20px;text-align:center;margin-bottom:28px;">
            <span style="font-size:40px;font-weight:800;color:#ffffff;letter-spacing:12px;">${otp}</span>
          </div>
          <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.35);line-height:1.5;">
            If you didn't request this, you can safely ignore this email.
          </p>
        </td></tr>
      </table>
      <p style="margin:20px 0 0;font-size:12px;color:rgba(255,255,255,0.2);">Series · Find your people on iMessage</p>
    </td></tr>
  </table>
</body>
</html>`.trim();

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), BREVO_TIMEOUT_MS);

  try {
    const res = await fetch("https://api.brevo.com/v3/smtp/email", {
      method: "POST",
      headers: { "Content-Type": "application/json", "api-key": apiKey },
      body: JSON.stringify({
        sender:      { email: senderEmail, name: "Series" },
        to:          [{ email: to }],
        subject:     `${otp} is your Series verification code`,
        htmlContent: html,
      }),
      signal: controller.signal,
    });

    if (!res.ok) {
      // Log only the status code — not the body — to avoid leaking provider internals
      throw new Error(`Brevo responded with status ${res.status}`);
    }
  } finally {
    clearTimeout(timer);
  }
}

// ── Routes ────────────────────────────────────────────────────────────────────

router.post("/auth/send-otp", async (req, res) => {
  const parsed = SendOtpBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const email = parsed.data.email.trim().toLowerCase();

  if (!EMAIL_RE.test(email)) {
    res.status(400).json({ error: "Please enter a valid email address." });
    return;
  }

  if (isRateLimited(sendRateByEmail, email, EMAIL_WINDOW_MS, EMAIL_MAX_SENDS)) {
    res.status(429).json({ error: "Too many OTP requests for this email. Please wait 10 minutes." });
    return;
  }

  const ip = (req.headers["x-forwarded-for"] as string | undefined)?.split(",")[0].trim()
    ?? req.socket.remoteAddress
    ?? "unknown";

  if (isRateLimited(sendRateByIp, ip, IP_WINDOW_MS, IP_MAX_SENDS)) {
    res.status(429).json({ error: "Too many requests. Please slow down." });
    return;
  }

  const otp = generateOtp();
  otpStore.set(email, { otp, expiresAt: Date.now() + OTP_TTL_MS, attempts: 0 });

  if (isBrevoConfigured()) {
    try {
      await sendOtpEmail(email, otp);
      req.log.info({ email }, "OTP email sent via Brevo");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      req.log.error({ email, brevoError: message }, "Failed to send OTP email");
      // Refund both rate-limit slots so a provider outage doesn't lock out the user
      const emailBucket = sendRateByEmail.get(email);
      if (emailBucket && emailBucket.count > 0) emailBucket.count -= 1;
      const ipBucket = sendRateByIp.get(ip);
      if (ipBucket && ipBucket.count > 0) ipBucket.count -= 1;
      // Remove the stored OTP — it was never delivered
      otpStore.delete(email);
      res.status(502).json({ error: "Failed to send verification email. Please try again." });
      return;
    }
    res.json({ success: true, message: `Verification code sent to ${email}`, devOtp: null });
  } else if (process.env.NODE_ENV !== "production") {
    // Brevo not configured — dev/local fallback only: expose OTP in response
    req.log.warn({ email }, "Brevo not configured; returning devOtp in response (dev only)");
    res.json({ success: true, message: `Verification code sent to ${email}`, devOtp: otp });
  } else {
    // Production with no email provider — fail closed
    otpStore.delete(email);
    req.log.error("Brevo not configured in production; rejecting send-otp request");
    res.status(503).json({ error: "Email delivery is not configured. Please contact support." });
  }
});

router.post("/auth/verify-otp", (req, res) => {
  const parsed = VerifyOtpBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const email    = parsed.data.email.trim().toLowerCase();
  const { otp }  = parsed.data;

  const entry = otpStore.get(email);
  if (!entry) {
    res.status(400).json({ error: "No OTP found for this email. Please request a new one." });
    return;
  }

  if (Date.now() > entry.expiresAt) {
    otpStore.delete(email);
    res.status(400).json({ error: "OTP has expired. Please request a new one." });
    return;
  }

  entry.attempts += 1;
  if (entry.attempts > MAX_VERIFY_ATTEMPTS) {
    otpStore.delete(email);
    res.status(400).json({ error: "Too many incorrect attempts. Please request a new OTP." });
    return;
  }

  if (otp !== entry.otp) {
    const remaining = MAX_VERIFY_ATTEMPTS - entry.attempts;
    res.status(400).json({
      error: `Incorrect OTP. ${remaining} attempt${remaining === 1 ? "" : "s"} remaining.`,
    });
    return;
  }

  otpStore.delete(email);

  // Issue a server-bound verification token so profile endpoints can derive
  // caller identity without trusting client-supplied email values.
  const token = randomBytes(32).toString("hex");
  const expiresAt = Date.now() + VERIFIED_TTL_MS;
  verificationTokens.set(token, { email, expiresAt });

  // Also record in the email-keyed store so /auth/send-welcome still works.
  verifiedEmails.set(email, expiresAt);

  req.log.info({ email }, "OTP verified successfully");
  res.json({ success: true, message: "Email verified successfully!", verificationToken: token });
});

// ── Welcome email ─────────────────────────────────────────────────────────────

async function sendWelcomeEmail(to: string, name: string, about: string): Promise<void> {
  const apiKey      = process.env.BREVO_API_KEY!;
  const senderEmail = process.env.BREVO_SENDER_EMAIL!;

  // Escape all user-controlled values before HTML interpolation
  const safeName      = escapeHtml(name.trim());
  const safeAbout     = escapeHtml(about.trim());
  const safeEmail     = escapeHtml(to);
  const safeFirstName = escapeHtml(name.trim().split(/\s+/)[0]);

  const aboutBlock = safeAbout
    ? `<p style="margin:0 0 4px;font-size:11px;font-weight:600;color:rgba(255,255,255,0.35);text-transform:uppercase;letter-spacing:0.08em;">About</p>
       <p style="margin:0 0 16px;font-size:14px;color:rgba(255,255,255,0.7);line-height:1.6;font-style:italic;">&ldquo;${safeAbout}&rdquo;</p>`
    : "";

  const steps: [string, string, string][] = [
    ["📱", "Download Series", "Available on the App Store for iPhone."],
    ["🔗", "Connect on iMessage", "Find people who share your interests — no feed, no follower counts."],
    ["✨", "Be yourself", "No scrolling. No vanity. Just real people, real conversations."],
  ];

  const html = `
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#0f0a07;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#0f0a07;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%" style="max-width:480px;">

        <!-- Header -->
        <tr><td style="padding-bottom:28px;text-align:center;">
          <span style="font-size:28px;font-weight:900;color:#ffffff;letter-spacing:-1px;">s_</span>
        </td></tr>

        <!-- Card -->
        <tr><td style="background:#1a0a06;border:1px solid rgba(255,255,255,0.1);border-radius:16px;padding:40px 36px;">

          <p style="margin:0 0 6px;font-size:24px;font-weight:800;color:#ffffff;letter-spacing:-0.5px;">
            Welcome to Series, ${safeFirstName} &#x1F44B;
          </p>
          <p style="margin:0 0 28px;font-size:14px;color:rgba(255,255,255,0.5);line-height:1.5;">
            Your profile is live. Here&rsquo;s a quick look at what you set up:
          </p>

          <!-- Profile summary -->
          <table width="100%" cellpadding="0" cellspacing="0"
            style="background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.1);border-radius:12px;padding:20px;margin-bottom:28px;">
            <tr><td>
              <p style="margin:0 0 4px;font-size:11px;font-weight:600;color:rgba(255,255,255,0.35);text-transform:uppercase;letter-spacing:0.08em;">Name</p>
              <p style="margin:0 0 16px;font-size:16px;font-weight:700;color:#ffffff;">${safeName}</p>
              ${aboutBlock}
              <p style="margin:0 0 4px;font-size:11px;font-weight:600;color:rgba(255,255,255,0.35);text-transform:uppercase;letter-spacing:0.08em;">Email</p>
              <p style="margin:0;font-size:14px;color:rgba(255,255,255,0.6);">${safeEmail}</p>
            </td></tr>
          </table>

          <!-- What's next -->
          <p style="margin:0 0 16px;font-size:15px;font-weight:700;color:#ffffff;">What&rsquo;s next</p>
          <table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:28px;">
            ${steps.map(([icon, title, desc]) => `
            <tr><td style="padding-bottom:16px;">
              <table cellpadding="0" cellspacing="0"><tr>
                <td style="width:36px;vertical-align:top;padding-top:2px;font-size:18px;">${icon}</td>
                <td>
                  <p style="margin:0 0 2px;font-size:14px;font-weight:700;color:#ffffff;">${escapeHtml(title)}</p>
                  <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.45);line-height:1.4;">${escapeHtml(desc)}</p>
                </td>
              </tr></table>
            </td></tr>`).join("")}
          </table>

          <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.3);line-height:1.5;border-top:1px solid rgba(255,255,255,0.08);padding-top:20px;">
            You&rsquo;re receiving this because you signed up for Series.<br>
            If this wasn&rsquo;t you, you can safely ignore this email.
          </p>
        </td></tr>

        <!-- Footer -->
        <tr><td style="padding-top:20px;text-align:center;">
          <p style="margin:0;font-size:12px;color:rgba(255,255,255,0.2);">Series &middot; Find your people on iMessage</p>
        </td></tr>

      </table>
    </td></tr>
  </table>
</body>
</html>`.trim();

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), BREVO_TIMEOUT_MS);

  try {
    const res = await fetch("https://api.brevo.com/v3/smtp/email", {
      method: "POST",
      headers: { "Content-Type": "application/json", "api-key": apiKey },
      body: JSON.stringify({
        sender:      { email: senderEmail, name: "Series" },
        to:          [{ email: to, name: safeName }],
        subject:     `Welcome to Series, ${safeFirstName} 🎉`,
        htmlContent: html,
      }),
      signal: controller.signal,
    });

    if (!res.ok) {
      throw new Error(`Brevo responded with status ${res.status}`);
    }
  } finally {
    clearTimeout(timer);
  }
}

router.post("/auth/send-welcome", async (req, res) => {
  const parsed = SendWelcomeBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const email = parsed.data.email.trim().toLowerCase();
  const name  = parsed.data.name.trim();
  const about = parsed.data.about?.trim() ?? "";

  if (!name) {
    res.status(400).json({ error: "Name is required." });
    return;
  }

  // Require proof of OTP verification — prevents arbitrary welcome-email spam
  const verifiedExpiry = verifiedEmails.get(email);
  if (!verifiedExpiry || Date.now() > verifiedExpiry) {
    res.status(403).json({ error: "Email not verified. Please complete OTP verification first." });
    return;
  }

  // Per-email rate limit: 2 welcome emails per hour
  if (isRateLimited(welcomeRateByEmail, email, WELCOME_EMAIL_WINDOW_MS, WELCOME_EMAIL_MAX)) {
    res.status(429).json({ error: "Too many requests for this email. Please try again later." });
    return;
  }

  // Per-IP rate limit
  const ip = (req.headers["x-forwarded-for"] as string | undefined)?.split(",")[0].trim()
    ?? req.socket.remoteAddress
    ?? "unknown";
  if (isRateLimited(welcomeRateByIp, ip, WELCOME_IP_WINDOW_MS, WELCOME_IP_MAX)) {
    res.status(429).json({ error: "Too many requests. Please slow down." });
    return;
  }

  // Consume the verification token — one welcome email per OTP flow
  verifiedEmails.delete(email);

  if (!isBrevoConfigured()) {
    if (process.env.NODE_ENV !== "production") {
      req.log.warn({ email }, "Brevo not configured; skipping welcome email (dev mode)");
      res.json({ success: true, message: "Welcome email skipped (dev mode)." });
    } else {
      req.log.error({ email }, "Brevo not configured in production; welcome email not sent");
      // Non-fatal in production too — user's profile is complete
      res.status(202).json({ success: true, message: "Profile saved. Welcome email could not be sent." });
    }
    return;
  }

  try {
    await sendWelcomeEmail(email, name, about);
    req.log.info({ email }, "Welcome email sent");
    res.json({ success: true, message: "Welcome email sent." });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, brevoError: message }, "Failed to send welcome email");
    // Non-fatal — profile is already saved; don't block the user
    res.status(202).json({ success: true, message: "Profile saved. Welcome email could not be sent." });
  }
});

export default router;
