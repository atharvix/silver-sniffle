import { Router, type IRouter, type Request, type Response } from "express";
import { eq } from "drizzle-orm";
import { db, profilesTable } from "@workspace/db";
import {
  UpsertProfileBody,
  UpdateLocationBody,
  GoOfflineBody,
} from "@workspace/api-zod";
import { getEmailFromToken } from "./auth";

// Presence TTL: a profile only shows up in others' nearby results if its
// heartbeat (set on every location update / heartbeat ping) is fresher than
// this. The frontend pings well inside this window (~6-7s) so there's
// comfortable margin for network jitter before someone flickers offline.
const PRESENCE_TTL_MS = 20_000;

function isPresent(lastSeenAt: Date | null): boolean {
  return lastSeenAt != null && Date.now() - lastSeenAt.getTime() <= PRESENCE_TTL_MS;
}

// ── OpenAI client (lazy, Replit-proxy-first) ──────────────────────────────────
// Uses Replit's managed AI proxy when AI_INTEGRATIONS_OPENAI_BASE_URL is set,
// otherwise falls back to a direct key via OPENAI_API_KEY.
// Returns null when neither is configured — callers fall back to trimmed bio.
function getOpenAIClient(): OpenAI | null {
  const baseURL = process.env.AI_INTEGRATIONS_OPENAI_BASE_URL;
  const proxyKey = process.env.AI_INTEGRATIONS_OPENAI_API_KEY;
  const directKey = process.env.OPENAI_API_KEY;

  if (baseURL && proxyKey) {
    return new OpenAI({ apiKey: proxyKey, baseURL });
  }
  if (directKey) {
    return new OpenAI({ apiKey: directKey });
  }
  return null;
}

const router: IRouter = Router();

// ── Auth helper ────────────────────────────────────────────────────────────────

/**
 * Extracts and validates the Bearer verification token from the request.
 * Returns the caller's email on success, or sends a 401 and returns null.
 */
async function requireToken(req: Request, res: Response): Promise<string | null> {
  const authHeader = req.headers.authorization;
  const token =
    authHeader?.startsWith("Bearer ") ? authHeader.slice(7).trim() : null;

  if (!token) {
    res.status(401).json({ error: "Authorization token required." });
    return null;
  }

  const email = await getEmailFromToken(token);
  if (!email) {
    res
      .status(401)
      .json({ error: "Verification token is invalid or has expired. Please verify your email again." });
    return null;
  }

  return email;
}

// ── Haversine distance (metres) ────────────────────────────────────────────────

const EARTH_RADIUS_M = 6_371_000;

function haversineMetres(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const toRad = (deg: number) => (deg * Math.PI) / 180;
  const dLat = toRad(lat2 - lat1);
  const dLon = toRad(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLon / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.sqrt(a));
}

// ── Routes ────────────────────────────────────────────────────────────────────

// Note: the larger JSON body limit for photo uploads on this route is
// applied in app.ts (mounted on the "/api/profiles" path prefix, before the
// app-wide default parser) — see the comment there for why ordering matters.
router.post("/profiles", async (req, res) => {
  const email = await requireToken(req, res);
  if (!email) return;

  const parsed = UpsertProfileBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const { name, about = "", photo = "" } = parsed.data;

  if (!name.trim()) {
    res.status(400).json({ error: "Name is required." });
    return;
  }

  try {
    await db
      .insert(profilesTable)
      .values({
        email,
        name: name.trim(),
        about: about.trim(),
        photo,
        updatedAt: new Date(),
      })
      .onConflictDoUpdate({
        target: profilesTable.email,
        set: {
          name: name.trim(),
          about: about.trim(),
          photo,
          updatedAt: new Date(),
        },
      });

    req.log.info({ email }, "Profile upserted");
    res.json({ success: true, message: "Profile saved." });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to upsert profile");
    res.status(500).json({ error: "Failed to save profile. Please try again." });
  }
});

router.get("/profiles/me", async (req, res) => {
  const email = await requireToken(req, res);
  if (!email) return;

  try {
    const [profile] = await db
      .select()
      .from(profilesTable)
      .where(eq(profilesTable.email, email));

    if (!profile) {
      res.status(404).json({ error: "Profile not found. Please create a profile first." });
      return;
    }

    res.json({
      email: profile.email,
      name: profile.name,
      about: profile.about ?? "",
      photo: profile.photo ?? "",
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to fetch own profile");
    res.status(500).json({ error: "Failed to fetch profile. Please try again." });
  }
});

router.post("/profiles/location", async (req, res) => {
  const email = await requireToken(req, res);
  if (!email) return;

  const parsed = UpdateLocationBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const { latitude, longitude } = parsed.data;

  try {
    const now = new Date();
    const result = await db
      .update(profilesTable)
      .set({ latitude, longitude, lastSeenAt: now, updatedAt: now })
      .where(eq(profilesTable.email, email))
      .returning({ email: profilesTable.email });

    if (result.length === 0) {
      res.status(404).json({ error: "Profile not found. Please create a profile first." });
      return;
    }

    req.log.info({ email, latitude, longitude }, "Location updated");
    res.json({ success: true, message: "Location updated." });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to update location");
    res.status(500).json({ error: "Failed to update location. Please try again." });
  }
});

router.post("/profiles/heartbeat", async (req, res) => {
  const email = await requireToken(req, res);
  if (!email) return;

  try {
    const result = await db
      .update(profilesTable)
      .set({ lastSeenAt: new Date() })
      .where(eq(profilesTable.email, email))
      .returning({ email: profilesTable.email });

    if (result.length === 0) {
      res.status(404).json({ error: "Profile not found. Please create a profile first." });
      return;
    }

    res.json({ success: true });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to record heartbeat");
    res.status(500).json({ error: "Failed to record heartbeat. Please try again." });
  }
});

// Token passed in the JSON body (not an Authorization header) because
// navigator.sendBeacon — used to fire this on tab close/unload — cannot set
// custom headers.
router.post("/profiles/offline", async (req, res) => {
  const parsed = GoOfflineBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: "Invalid request body" });
    return;
  }

  const email = await getEmailFromToken(parsed.data.token);
  if (!email) {
    res.status(401).json({ error: "Verification token is invalid or has expired." });
    return;
  }

  try {
    await db
      .update(profilesTable)
      .set({ lastSeenAt: null })
      .where(eq(profilesTable.email, email));

    req.log.info({ email }, "Profile marked offline");
    res.json({ success: true });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to mark profile offline");
    res.status(500).json({ error: "Failed to go offline." });
  }
});

const NEARBY_RADIUS_M = 30;

router.get("/profiles/nearby", async (req, res) => {
  const email = await requireToken(req, res);
  if (!email) return;

  try {
    // Fetch all profiles in one query; event-venue proximity means small datasets
    const allProfiles = await db.select().from(profilesTable);

    const caller = allProfiles.find((p) => p.email === email);
    if (!caller) {
      res.status(404).json({ error: "Profile not found. Please create a profile first." });
      return;
    }

    if (caller.latitude == null || caller.longitude == null) {
      res.status(400).json({
        error: "No location stored for your profile. Please update your location first.",
      });
      return;
    }

    if (!isPresent(caller.lastSeenAt)) {
      res.status(400).json({
        error: "Your location sharing has expired. Please share your location again.",
      });
      return;
    }

    const callerLat = caller.latitude;
    const callerLon = caller.longitude;

    // Filter to profiles that are within radius AND currently present
    // (fresh heartbeat) — excludes the caller, and anyone who closed the
    // tab, backgrounded it, or otherwise went stale.
    const nearby = allProfiles
      .filter(
        (p) =>
          p.email !== email &&
          p.latitude != null &&
          p.longitude != null &&
          isPresent(p.lastSeenAt) &&
          haversineMetres(callerLat, callerLon, p.latitude, p.longitude) <= NEARBY_RADIUS_M,
      )
      .sort(
        (a, b) =>
          haversineMetres(callerLat, callerLon, a.latitude!, a.longitude!) -
          haversineMetres(callerLat, callerLon, b.latitude!, b.longitude!),
      );

    const profiles = nearby.map((p) => {
      const about = p.about?.trim() ?? "";
      return {
        name: p.name,
        photo: p.photo,
        distanceMeters: haversineMetres(callerLat, callerLon, p.latitude!, p.longitude!),
        headline: about,
        conversationStarter: about,
      };
    });

    res.json({ profiles });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to fetch nearby profiles");
    res.status(500).json({ error: "Failed to fetch nearby profiles. Please try again." });
  }
});

export default router;
