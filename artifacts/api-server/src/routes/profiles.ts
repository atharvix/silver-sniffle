import { Router, type IRouter, type Request, type Response } from "express";
import { eq } from "drizzle-orm";
import OpenAI from "openai";
import { db, profilesTable } from "@workspace/db";
import {
  UpsertProfileBody,
  UpdateLocationBody,
} from "@workspace/api-zod";
import { getEmailFromToken } from "./auth";

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

// ── AI conversation-starter summary ──────────────────────────────────────────

const SUMMARY_MAX_CHARS = 140;

async function generateConversationStarter(about: string): Promise<string> {
  const client = getOpenAIClient();
  if (!client) return trimBio(about);

  try {
    const response = await client.chat.completions.create({
      model: "gpt-4o-mini",
      max_tokens: 60,
      messages: [
        {
          role: "system",
          content:
            "You write one short, friendly, natural conversation-starter line (max 20 words) " +
            "based on someone's bio. The line should give a stranger an easy, specific opening " +
            "to start a conversation. Return only the line, no quotes, no punctuation at the end.",
        },
        { role: "user", content: about },
      ],
    });
    return response.choices[0]?.message?.content?.trim() ?? trimBio(about);
  } catch {
    return trimBio(about);
  }
}

function trimBio(about: string): string {
  const trimmed = about.trim();
  if (!trimmed) return "";
  if (trimmed.length <= SUMMARY_MAX_CHARS) return trimmed;
  return trimmed.slice(0, SUMMARY_MAX_CHARS - 1) + "…";
}

// ── Ensure AI summary is up to date ───────────────────────────────────────────

async function ensureSummary(
  profile: typeof profilesTable.$inferSelect,
): Promise<string> {
  const about = profile.about ?? "";

  // Return cached summary if the bio hasn't changed
  if (profile.aiSummary && profile.aiSummaryAbout === about) {
    return profile.aiSummary;
  }

  const summary = await generateConversationStarter(about);

  // Persist the new summary — don't block the response on a DB write error
  db.update(profilesTable)
    .set({ aiSummary: summary, aiSummaryAbout: about })
    .where(eq(profilesTable.email, profile.email))
    .catch((err: unknown) => {
      const message = err instanceof Error ? err.message : String(err);
      console.error({ email: profile.email, err: message }, "Failed to cache AI summary");
    });

  return summary;
}

// ── Routes ────────────────────────────────────────────────────────────────────

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
          // Reset cached AI summary so it regenerates for the new bio
          aiSummary: null,
          aiSummaryAbout: null,
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
    const result = await db
      .update(profilesTable)
      .set({ latitude, longitude, updatedAt: new Date() })
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

    const callerLat = caller.latitude;
    const callerLon = caller.longitude;

    // Filter to profiles within radius, excluding the caller
    const nearby = allProfiles
      .filter(
        (p) =>
          p.email !== email &&
          p.latitude != null &&
          p.longitude != null &&
          haversineMetres(callerLat, callerLon, p.latitude, p.longitude) <= NEARBY_RADIUS_M,
      )
      .sort(
        (a, b) =>
          haversineMetres(callerLat, callerLon, a.latitude!, a.longitude!) -
          haversineMetres(callerLat, callerLon, b.latitude!, b.longitude!),
      );

    // Generate / retrieve AI summaries in parallel
    const profiles = await Promise.all(
      nearby.map(async (p) => ({
        name: p.name,
        photo: p.photo,
        distanceMeters: haversineMetres(callerLat, callerLon, p.latitude!, p.longitude!),
        conversationStarter: await ensureSummary(p),
      })),
    );

    res.json({ profiles });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    req.log.error({ email, err: message }, "Failed to fetch nearby profiles");
    res.status(500).json({ error: "Failed to fetch nearby profiles. Please try again." });
  }
});

export default router;
