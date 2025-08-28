package main

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
)

type (
	MaskedEmail [32]byte

	Model struct {
		Ada struct {
			Emails       []string
			OurMasks     []MaskedEmail
			DoubleMasked []MaskedEmail
		}
		Theirs struct {
			Emails       []string
			OurMasks     []MaskedEmail
			DoubleMasked []MaskedEmail
		}
		Intersection []MaskedEmail
	}
)

func dedup[T comparable, E ~[]T](list E, cmp func(a, b T) int) E {
	set := map[T]struct{}{}
	for _, v := range list {
		set[v] = struct{}{}
	}
	var out E
	for v := range set {
		out = append(out, v)
	}
	slices.SortStableFunc(out, cmp)
	return out
}

func intersect[T comparable, E ~[]T](la, lb E) E {
	sa := map[T]struct{}{}
	sb := map[T]struct{}{}
	for _, v := range la {
		sa[v] = struct{}{}
	}
	for _, v := range lb {
		sb[v] = struct{}{}
	}
	var out E
	if len(sa) > len(sb) {
		sa, sb = sb, sa
	}
	for v := range sa {
		if _, ok := sb[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

func (m *Model) UnmarshalValues(vals url.Values) error {
	// helper: take all raw values for a key, split them by newlines, trim and filter empties
	splitLines := func(key string) []string {
		out := []string{}
		for _, v := range vals[key] {
			for _, line := range strings.Split(v, "\n") {
				s := strings.TrimSpace(line)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}

	// Ada emails
	m.Ada.Emails = append(m.Ada.Emails, splitLines("ada.emails")...)
	m.Ada.Emails = dedup(m.Ada.Emails, strings.Compare)

	// Ada our masks
	for _, me := range splitLines("ada.our_masks") {
		var maskedEmail MaskedEmail
		if err := maskedEmail.FromURL(me); err != nil {
			return err
		}
		m.Ada.OurMasks = append(m.Ada.OurMasks, maskedEmail)
	}
	m.Ada.OurMasks = dedup(m.Ada.OurMasks, MaskedEmail.Compare)

	// Ada double masked
	for _, me := range splitLines("ada.double_masked") {
		var maskedEmail MaskedEmail
		if err := maskedEmail.FromURL(me); err != nil {
			return err
		}
		m.Ada.DoubleMasked = append(m.Ada.DoubleMasked, maskedEmail)
	}
	m.Ada.DoubleMasked = dedup(m.Ada.DoubleMasked, MaskedEmail.Compare)

	// Theirs emails
	m.Theirs.Emails = append(m.Theirs.Emails, splitLines("theirs.emails")...)
	m.Theirs.Emails = dedup(m.Theirs.Emails, strings.Compare)

	// Theirs our masks
	for _, me := range splitLines("theirs.our_masks") {
		var maskedEmail MaskedEmail
		if err := maskedEmail.FromURL(me); err != nil {
			return err
		}
		m.Theirs.OurMasks = append(m.Theirs.OurMasks, maskedEmail)
	}
	m.Theirs.OurMasks = dedup(m.Theirs.OurMasks, MaskedEmail.Compare)

	// Theirs double masked
	for _, me := range splitLines("theirs.double_masked") {
		var maskedEmail MaskedEmail
		if err := maskedEmail.FromURL(me); err != nil {
			return err
		}
		m.Theirs.DoubleMasked = append(m.Theirs.DoubleMasked, maskedEmail)
	}
	m.Theirs.DoubleMasked = dedup(m.Theirs.DoubleMasked, MaskedEmail.Compare)

	// Intersection
	for _, i := range splitLines("intersection") {
		var maskedEmail MaskedEmail
		if err := maskedEmail.FromURL(i); err != nil {
			return err
		}
		m.Intersection = append(m.Intersection, maskedEmail)
	}
	m.Intersection = dedup(m.Intersection, MaskedEmail.Compare)
	return nil
}

func (m *MaskedEmail) FromURL(val string) error {
	decoded, err := base64.URLEncoding.DecodeString(val)
	if err != nil {
		return err
	}
	if len(decoded) != len(m) {
		return errors.New("invalid masked email length")
	}
	copy(m[:], decoded)
	return nil
}

func (m *MaskedEmail) ToURL() string {
	return base64.URLEncoding.EncodeToString(m[:])
}

func (m MaskedEmail) String() string {
	return m.ToURL()
}

func (m MaskedEmail) Compare(o MaskedEmail) int {
	return bytes.Compare(m[:], o[:])
}

var (
	//go:embed index.html
	indexContent []byte
)

var (
	tmpl *template.Template = template.Must(template.New("__root__").Parse(string(indexContent)))
)

func main() {
	mux := http.NewServeMux()
	model := Model{}
	model.Ada.Emails = []string{"sentinel", "alice@example.com", "bob@example.com"}
	model.Theirs.Emails = []string{"sentinel", "bob@example.com", "charlie@example.com"}
	var rw sync.RWMutex
	var ourKey, theirKey [32]byte
	_, err := rand.Read(ourKey[:])
	if err != nil {
		panic(fmt.Sprintf("failed to read random: %v", err))
	}
	_, err = rand.Read(theirKey[:])
	if err != nil {
		panic(fmt.Sprintf("failed to read random: %v", err))
	}
	renderModel := func() ([]byte, error) {
		rw.RLock()
		defer rw.RUnlock()
		buf := bytes.Buffer{}
		if err := tmpl.Execute(&buf, model); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	mux.Handle("GET /psi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, err := renderModel()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to render model: %v", err), http.StatusInternalServerError)
			return
		}
		w.Write(buf)
	}))
	mux.Handle("POST /psi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var nm Model
		r.ParseForm()
		err := nm.UnmarshalValues(r.Form)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid input: %v", err), http.StatusBadRequest)
			return
		}
		switch r.FormValue("action") {
		case "update_our_masks":
			nm.Ada.OurMasks = FirstStep(ourKey, nm.Ada.Emails)
			nm.Theirs.OurMasks = FirstStep(theirKey, nm.Theirs.Emails)
		case "update_double_masked":
			nm.Ada.DoubleMasked = SecondStep(ourKey, nm.Theirs.OurMasks)
			nm.Theirs.DoubleMasked = SecondStep(theirKey, nm.Ada.OurMasks)
		case "compute_intersection":
			nm.Intersection = intersect(nm.Ada.DoubleMasked, nm.Theirs.DoubleMasked)
			slices.SortFunc(nm.Intersection, MaskedEmail.Compare)
		}
		rw.Lock()
		model = nm
		rw.Unlock()
		buf, err := renderModel()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to render model: %v", err), http.StatusInternalServerError)
			return
		}
		w.Write(buf)
	}))
	addr := fmt.Sprintf("%v:%v", os.Getenv("BIND_ADDR"), os.Getenv("BIND_PORT"))
	log.Printf("starting server at: %v", addr)
	http.ListenAndServe(addr, mux)
}
