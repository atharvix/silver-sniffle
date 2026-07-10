import { Router, type IRouter } from "express";
import { randomInt } from "crypto";
import { SendOtpBody, VerifyOtpBody } from "@workspace/api-zod";

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
const otpStore = new Map<string, OtpEntry>();
// Rate limiting: keyed by `phone:ip`
const sendRateByPhone = new Map<string, RateBucket>(); // per phone
const sendRateByIp   = new Map<string, RateBucket>(); // per IP

const INDIAN_PHONE_RE = /^[6-9]\d{9}$/;
const OTP_TTL_MS       = 10 * 60 * 1000;   // 10 minutes
const MAX_VERIFY_ATTEMPTS = 5;

// Rate-limiting windows for send-otp
const PHONE_WINDOW_MS  = 10 * 60 * 1000;   // 10-minute window
const PHONE_MAX_SENDS  = 3;                 // max 3 OTPs per phone per window
const IP_WINDOW_MS     = 60 * 1000;         // 1-minute window
const IP_MAX_SENDS     = 10;               // max 10 sends per IP per minute

function generateOtp(): string {
  // Cryptographically secure 4-digit OTP (1000–9999)
  return String(randomInt(1000, 10000));
}

function isRateLimited(
  store: Map<string, RateBucket>,
  key: string,
  windowMs: number,
  maxCount: number,
): boolean {
  const now = Date.now();
  const bucket = store.get(key);

  if (!bucket || now - bucket.windowStart > windowMs) {
    store.set(key, { count: 1, windowStart: now });
    return false;
  }

  if (bucket.count >= maxCount) return true;
  bucket.count += 1;
  return false;
}

router.post("/auth/send-otp", (req, res) => {
  const parsed = SendOtpBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const { phone } = parsed.data;

  if (!INDIAN_PHONE_RE.test(phone)) {
    res.status(400).json({
      error: "Please enter a valid 10-digit Indian mobile number starting with 6–9",
    });
    return;
  }

  // Per-phone rate limit: 3 OTPs per 10 minutes
  if (isRateLimited(sendRateByPhone, phone, PHONE_WINDOW_MS, PHONE_MAX_SENDS)) {
    res.status(429).json({
      error: "Too many OTP requests for this number. Please wait 10 minutes.",
    });
    return;
  }

  // Per-IP rate limit: 10 sends per minute
  const ip = (req.headers["x-forwarded-for"] as string | undefined)?.split(",")[0].trim()
    ?? req.socket.remoteAddress
    ?? "unknown";

  if (isRateLimited(sendRateByIp, ip, IP_WINDOW_MS, IP_MAX_SENDS)) {
    res.status(429).json({ error: "Too many requests. Please slow down." });
    return;
  }

  const otp = generateOtp();
  otpStore.set(phone, {
    otp,
    expiresAt: Date.now() + OTP_TTL_MS,
    attempts: 0,
  });

  // In production, send via SMS provider here.
  req.log.info({ phone: `+91${phone}` }, "OTP generated");

  res.json({
    success: true,
    message: `OTP sent to +91 ${phone}`,
    // Expose OTP only in development so the demo works without an SMS provider
    devOtp: process.env.NODE_ENV !== "production" ? otp : null,
  });
});

router.post("/auth/verify-otp", (req, res) => {
  const parsed = VerifyOtpBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const { phone, otp } = parsed.data;

  const entry = otpStore.get(phone);
  if (!entry) {
    res.status(400).json({
      error: "No OTP found for this number. Please request a new one.",
    });
    return;
  }

  if (Date.now() > entry.expiresAt) {
    otpStore.delete(phone);
    res.status(400).json({ error: "OTP has expired. Please request a new one." });
    return;
  }

  entry.attempts += 1;
  if (entry.attempts > MAX_VERIFY_ATTEMPTS) {
    otpStore.delete(phone);
    res.status(400).json({
      error: "Too many incorrect attempts. Please request a new OTP.",
    });
    return;
  }

  if (otp !== entry.otp) {
    const remaining = MAX_VERIFY_ATTEMPTS - entry.attempts;
    res.status(400).json({
      error: `Incorrect OTP. ${remaining} attempt${remaining === 1 ? "" : "s"} remaining.`,
    });
    return;
  }

  otpStore.delete(phone);
  req.log.info({ phone: `+91${phone}` }, "OTP verified successfully");

  res.json({ success: true, message: "Phone number verified successfully!" });
});

export default router;
