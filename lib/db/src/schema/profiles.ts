import { pgTable, text, doublePrecision, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema, createSelectSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const profilesTable = pgTable("profiles", {
  email: text("email").primaryKey(),
  name: text("name").notNull(),
  about: text("about").notNull().default(""),
  photo: text("photo").notNull().default(""),
  latitude: doublePrecision("latitude"),
  longitude: doublePrecision("longitude"),
  // Presence heartbeat: refreshed on every location update / heartbeat ping,
  // cleared immediately on explicit "go offline" (tab close). Nearby queries
  // only return profiles whose lastSeenAt is within the presence TTL, so
  // closing the app or walking away makes you disappear from others' results.
  lastSeenAt: timestamp("last_seen_at"),
  aiSummary: text("ai_summary"),
  aiSummaryAbout: text("ai_summary_about"),
  // AI-generated headline (short punchy identity tagline, e.g. "Product
  // designer building AI tools") shown prominently on the profile card.
  // Cached against the bio it was generated from, same pattern as aiSummary.
  headline: text("headline"),
  headlineAbout: text("headline_about"),
  createdAt: timestamp("created_at").defaultNow().notNull(),
  updatedAt: timestamp("updated_at").defaultNow().notNull(),
});

export const insertProfileSchema = createInsertSchema(profilesTable).omit({
  createdAt: true,
  updatedAt: true,
  aiSummary: true,
  aiSummaryAbout: true,
  headline: true,
  headlineAbout: true,
});

export const selectProfileSchema = createSelectSchema(profilesTable);

export type InsertProfile = z.infer<typeof insertProfileSchema>;
export type Profile = typeof profilesTable.$inferSelect;
