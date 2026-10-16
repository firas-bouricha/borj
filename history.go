package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Historique : un point toutes les 5 s (CSV journalier) + journal des pics >= 85 %.
// Dossier : %LOCALAPPDATA%\Borj

const (
	histEvery = 5 * time.Second
	longLen   = 720 // 1 h à 5 s
	peakKeep  = 50
)

type point struct{ cpu, ram, commit float64 }

type recorder struct {
	dir       string
	long      []point
	lastWrite time.Time

	inPeak    bool
	peakStart time.Time
	peakRAM   float64
	peakCom   float64
	peakTop   string

	peaks []string // plus récent en dernier
}

func newRecorder() *recorder {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	r := &recorder{dir: filepath.Join(base, "Borj")}
	_ = os.MkdirAll(r.dir, 0o755)
	if f, err := os.Open(r.peaksPath()); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			r.peaks = append(r.peaks, sc.Text())
		}
		f.Close()
		if len(r.peaks) > peakKeep {
			r.peaks = r.peaks[len(r.peaks)-peakKeep:]
		}
	}
	return r
}

func (r *recorder) peaksPath() string { return filepath.Join(r.dir, "pics.log") }

func (r *recorder) appendLine(path, header, line string) {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if os.IsNotExist(statErr) && header != "" {
		fmt.Fprintln(f, header)
	}
	fmt.Fprintln(f, line)
}

// record : appelé à chaque tick ; renvoie un texte quand un pic se termine.
func (r *recorder) record(s Snapshot, top string) string {
	now := time.Now()
	ram, com := s.MemUsedPct(), s.CommitPct()

	if now.Sub(r.lastWrite) >= histEvery {
		r.lastWrite = now
		r.long = append(r.long, point{s.CPUTotal, ram, com})
		if len(r.long) > longLen {
			r.long = r.long[len(r.long)-longLen:]
		}
		csv := filepath.Join(r.dir, "historique-"+now.Format("2006-01-02")+".csv")
		r.appendLine(csv, "heure;cpu_pct;ram_pct;commit_pct;ram_utilisee_go;top_apps",
			fmt.Sprintf("%s;%.0f;%.0f;%.0f;%.1f;%s", now.Format("15:04:05"), s.CPUTotal, ram, com,
				float64(s.MemTotal-s.MemAvail)/(1<<30), top))
	}

	switch {
	case !r.inPeak && (ram >= peakOn || com >= peakOn):
		r.inPeak, r.peakStart, r.peakRAM, r.peakCom, r.peakTop = true, now, ram, com, top
	case r.inPeak && (ram >= peakOff || com >= peakOff):
		if ram > r.peakRAM {
			r.peakRAM, r.peakTop = ram, top
		}
		r.peakCom = max(r.peakCom, com)
	case r.inPeak:
		r.inPeak = false
		line := fmt.Sprintf("%s  %s→%s  RAM max %.0f%%  commit max %.0f%%  |  %s",
			r.peakStart.Format("02/01"), r.peakStart.Format("15:04"), now.Format("15:04"), r.peakRAM, r.peakCom, r.peakTop)
		r.appendLine(r.peaksPath(), "", line)
		r.peaks = append(r.peaks, line)
		if len(r.peaks) > peakKeep {
			r.peaks = r.peaks[1:]
		}
		return "Pic terminé, enregistré dans l'historique"
	}
	return ""
}

func (r *recorder) series(f func(point) float64) []float64 {
	out := make([]float64, len(r.long))
	for i, p := range r.long {
		out[i] = f(p)
	}
	return out
}
