package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// Hand-over on restart: positions live only in memory, so a deploy used to drop everyone
// until their phones reported again (a still phone asleep in Doze could take many minutes).
// On shutdown the hub writes its fresh fixes to RAM (/dev/shm, never disk), sealed with
// DATA_KEY; the next process reads them back if they're under handoverMaxAge old and deletes
// the file at once. Then it wakes those phones so their sockets come back.
const handoverMaxAge = 2 * time.Minute

func handoverPath() string {
	if p := os.Getenv("HUB_HANDOVER"); p != "" {
		return p
	}
	return "/dev/shm/kinjo-hub.handover"
}

type savedFix struct {
	UID   string    `json:"u"`
	Lat   float64   `json:"lat"`
	Lng   float64   `json:"lng"`
	Acc   int       `json:"c"`
	T     time.Time `json:"t"`
	At    time.Time `json:"at"`
	Still bool      `json:"s,omitempty"`
	VN    float64   `json:"vn,omitempty"`
	VE    float64   `json:"ve,omitempty"`
	Wifi  []uint32  `json:"w,omitempty"`
}

type handover struct {
	Saved time.Time  `json:"saved"`
	Fixes []savedFix `json:"fixes"`
}

// saveHandover writes the fresh fixes for the next process. Called once, on shutdown.
func (h *Hub) saveHandover(path string) {
	now := time.Now()
	h.mu.Lock()
	out := handover{Saved: now}
	for uid, f := range h.fixes {
		if f.fresh(now) && !h.hidden[uid] {
			out.Fixes = append(out.Fixes, savedFix{UID: uid, Lat: f.lat, Lng: f.lng, Acc: f.acc, T: f.t, At: f.at,
				Still: f.still, VN: f.vn, VE: f.ve, Wifi: f.wifi})
		}
	}
	h.mu.Unlock()
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	sealed, err := seal(string(b))
	if err != nil {
		log.Printf("handover: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(sealed), 0o600); err != nil {
		log.Printf("handover: %v", err)
		return
	}
	log.Printf("handover: saved %d positions", len(out.Fixes))
}

// loadHandover reads and deletes the last process's fixes. Returns who came back.
func (h *Hub) loadHandover(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	_ = os.Remove(path) // read once: positions never outlive the hand-over
	plain, err := unseal(string(raw))
	if err != nil {
		log.Printf("handover: %v", err)
		return nil
	}
	var in handover
	if json.Unmarshal([]byte(plain), &in) != nil || time.Since(in.Saved) > handoverMaxAge {
		return nil
	}
	now := time.Now()
	var uids []string
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range in.Fixes {
		if h.hidden[s.UID] {
			continue
		}
		f := &fix{lat: s.Lat, lng: s.Lng, acc: s.Acc, t: s.T, at: s.At, still: s.Still, vn: s.VN, ve: s.VE,
			wifi: s.Wifi, cell: geohash(s.Lat, s.Lng)}
		if f.still {
			f.at = now // its socket is reconnecting: hold it a full stillFresh meanwhile
		}
		if !f.fresh(now) {
			continue
		}
		h.fixes[s.UID] = f
		if h.cells[f.cell] == nil {
			h.cells[f.cell] = map[string]bool{}
		}
		h.cells[f.cell][s.UID] = true
		h.areas[f.cell[:ghPrec-1]]++
		uids = append(uids, s.UID)
	}
	log.Printf("handover: restored %d positions", len(uids))
	return uids
}

// wakeRestored nudges the restored phones to reconnect, 100 a second so they don't all dial at once.
func (h *Hub) wakeRestored(uids []string) {
	if h.wake == nil {
		return
	}
	for i := 0; i < len(uids); i += 100 {
		h.wake(uids[i:min(i+100, len(uids))])
		time.Sleep(time.Second)
	}
}
