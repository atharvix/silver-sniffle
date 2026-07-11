import express, { type Express } from "express";
import cors from "cors";
import pinoHttp from "pino-http";
import router from "./routes";
import { logger } from "./lib/logger";

const app: Express = express();

app.use(
  pinoHttp({
    logger,
    serializers: {
      req(req) {
        return {
          id: req.id,
          method: req.method,
          url: req.url?.split("?")[0],
        };
      },
      res(res) {
        return {
          statusCode: res.statusCode,
        };
      },
    },
  }),
);
app.use(cors());

// Profile photo uploads are sent as base64 data URLs (~33% larger than the
// raw file, up to 8 MB client-side) — this path gets a scoped larger body
// limit. It must be registered *before* the general parser below: body-parser
// skips re-parsing once a request body has already been consumed, so mounting
// order determines which limit actually applies to a given path.
app.use("/api/profiles", express.json({ limit: "12mb" }));

// Default JSON/urlencoded body limit stays small for every other route (auth,
// location, etc. never need more than a few KB), keeping the request-size
// attack surface minimal everywhere except the profile photo upload path.
app.use(express.json({ limit: "256kb" }));
app.use(express.urlencoded({ extended: true, limit: "256kb" }));

app.use("/api", router);

export default app;
