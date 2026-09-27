/**
 * Self-check for LiveHumanTracker liveness.
 *
 * Run: npm run check:blink
 *
 * Guards the two failure modes that actually shipped:
 *   1. a real blink must register (the eye band has to be narrow enough that the
 *      blink moves its mean past the threshold), and
 *   2. a face that never blinks must NOT (a printed photo or a still must not
 *      pass the liveness gate just by being present).
 */
import { LiveHumanTracker } from '../src/utils/faceDetector.ts';

const VW = 480;
const VH = 640;

const BG: [number, number, number] = [40, 40, 45];
const SKIN: [number, number, number] = [200, 160, 130];
const PUPIL: [number, number, number] = [25, 25, 30];

// Synthetic portrait camera frame: skin oval plus two dark pupils on the eye
// line. Geometry is in source pixels; the tracker centre-crops this to 4:5 and
// scales by 0.5, so source y≈260 lands in the middle of the analysis eye band.
function makeFrame(pupilsOpen: boolean, shift = 0): Uint8ClampedArray {
  const px = new Uint8ClampedArray(VW * VH * 4);
  const put = (x: number, y: number, c: [number, number, number]) => {
    const i = (y * VW + x) * 4;
    px[i] = c[0];
    px[i + 1] = c[1];
    px[i + 2] = c[2];
    px[i + 3] = 255;
  };

  const cx = VW / 2 + shift;
  const cy = VH / 2 + 60;
  const eyeY = Math.round(cy - 100);

  for (let y = 0; y < VH; y++) {
    for (let x = 0; x < VW; x++) {
      if (((x - cx) / 140) ** 2 + ((y - cy) / 180) ** 2 <= 1) {
        put(x, y, SKIN);
        continue;
      }
      put(x, y, BG);
    }
  }

  // Open eyes: dark pupils. A blink replaces them with eyelid skin, which is the
  // signal the detector keys on.
  if (pupilsOpen) {
    for (let y = eyeY - 12; y <= eyeY + 12; y++) {
      for (let x = Math.round(cx - 62); x <= Math.round(cx + 62); x++) {
        for (const ex of [cx - 45, cx + 45]) {
          if (((x - ex) / 16) ** 2 + ((y - eyeY) / 10) ** 2 <= 1) put(x, y, PUPIL);
        }
      }
    }
  }
  return px;
}

/** Minimal stand-in for the parts of HTMLCanvasElement the tracker touches. */
class FakeCanvas {
  width = 0;
  height = 0;
  private buf = new Uint8ClampedArray(0);
  private frame = new Uint8ClampedArray(0);

  load(frame: Uint8ClampedArray) {
    this.frame = frame;
  }

  getContext() {
    return {
      drawImage: (
        _video: unknown,
        sx: number,
        sy: number,
        sw: number,
        sh: number,
        dx: number,
        dy: number,
        dw: number,
        dh: number
      ) => {
        this.buf = new Uint8ClampedArray(this.width * this.height * 4);
        for (let y = 0; y < dh; y++) {
          for (let x = 0; x < dw; x++) {
            const sxr = Math.min(VW - 1, sx + Math.floor((x * sw) / dw));
            const syr = Math.min(VH - 1, sy + Math.floor((y * sh) / dh));
            const s = (syr * VW + sxr) * 4;
            const d = ((y + dy) * this.width + (x + dx)) * 4;
            this.buf[d] = this.frame[s];
            this.buf[d + 1] = this.frame[s + 1];
            this.buf[d + 2] = this.frame[s + 2];
            this.buf[d + 3] = 255;
          }
        }
      },
      getImageData: (x: number, y: number, w: number, h: number) => {
        const out = new Uint8ClampedArray(w * h * 4);
        for (let row = 0; row < h; row++) {
          for (let col = 0; col < w; col++) {
            const s = ((y + row) * this.width + (x + col)) * 4;
            const d = (row * w + col) * 4;
            out[d] = this.buf[s];
            out[d + 1] = this.buf[s + 1];
            out[d + 2] = this.buf[s + 2];
            out[d + 3] = 255;
          }
        }
        return { data: out };
      },
    };
  }
}

function run(frames: Uint8ClampedArray[]) {
  const canvas = new FakeCanvas();
  const video = { readyState: 4, videoWidth: VW, videoHeight: VH } as unknown as HTMLVideoElement;
  const tracker = new LiveHumanTracker();
  return frames.map((frame) => {
    canvas.load(frame);
    return tracker.processFrame(canvas as unknown as HTMLCanvasElement, video);
  });
}

const fail = (msg: string, extra = '') => {
  console.error(`FAIL: ${msg}${extra ? `\n${extra}` : ''}`);
  process.exit(1);
};

// 1. A real blink is accepted.
const withBlink = [
  ...Array.from({ length: 12 }, () => makeFrame(true)),
  makeFrame(false), // blink
  makeFrame(false),
  ...Array.from({ length: 12 }, () => makeFrame(true)),
];
const blinkStatuses = run(withBlink);
if (!blinkStatuses.some((s) => s.hasBlinked)) {
  fail(
    'a real blink was never detected (this is the shipped bug)',
    `phases: ${blinkStatuses.map((s) => s.phase).join(',')}`
  );
}

// 2. A face that never blinks is rejected, even after a long stare.
const noBlink = run(Array.from({ length: 80 }, () => makeFrame(true)));
if (noBlink.some((s) => s.hasBlinked) || noBlink.some((s) => s.isHuman)) {
  fail('a static face passed the liveness gate without blinking');
}

// 3. Blink plus movement reaches a full verification.
const verified = run([
  ...withBlink,
  ...Array.from({ length: 40 }, (_, i) => makeFrame(true, (i % 2 === 0 ? 1 : -1) * 12)),
]);
if (!verified.some((s) => s.isHuman && s.progress >= 100)) {
  fail('blink + head motion never reached verified', JSON.stringify(verified.at(-1)));
}

console.log('ok: blink detected, still face rejected, blink+motion verifies');
