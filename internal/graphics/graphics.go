package graphics

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

const MaxImages = 48

// sourceBudget bounds downloaded image bytes kept for redrawing at other sizes.
const sourceBudget = 32 << 20

type Request struct {
	URL        string
	Cols, Rows int
	Ring       string
	Icon       string
}
type Job struct {
	Request Request
	ID      int
	ctx     context.Context
	avatar  *image.NRGBA
}
type Result struct {
	Job    Job
	Raw    string
	Err    error
	avatar *image.NRGBA
}
type entry struct {
	id            int
	ready, failed bool
	cancel        context.CancelFunc
	avatar        *image.NRGBA
}

type avatarKey struct{ url, ring, icon string }

type sourceCache struct {
	mu    sync.Mutex
	data  map[string][]byte
	order []string
	size  int
}

func (c *sourceCache) get(url string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.data[url]
	if ok {
		c.touch(url)
	}
	return data
}

func (c *sourceCache) put(url string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(data) > sourceBudget {
		return
	}
	if c.data == nil {
		c.data = make(map[string][]byte)
	}
	if old, ok := c.data[url]; ok {
		c.size -= len(old)
	}
	c.data[url] = data
	c.size += len(data)
	c.touch(url)
	for c.size > sourceBudget {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.size -= len(c.data[oldest])
		delete(c.data, oldest)
	}
}

func (c *sourceCache) touch(url string) {
	for i, key := range c.order {
		if key == url {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, url)
}

func (c *sourceCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data, c.order, c.size = nil, nil, 0
}

type Manager struct {
	Enabled, Tmux bool
	ProbeID       int
	next          int
	origin        *url.URL
	client        *http.Client
	slots         chan struct{}
	entries       map[Request]*entry
	sources       sourceCache
}

func New(origin string) *Manager {
	u, _ := url.Parse(origin)
	var seed [4]byte
	_, _ = rand.Read(seed[:])
	base := int(binary.BigEndian.Uint32(seed[:])&0x7fffff) + 0x10000
	m := &Manager{origin: u, ProbeID: base, next: base, entries: make(map[Request]*entry), slots: make(chan struct{}, 2)}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = m.dial
	m.client = &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !m.validURL(req.URL) {
			return errors.New("unsafe image redirect")
		}
		return nil
	}}
	return m
}

func (m *Manager) Probe() string {
	return m.wrap(ansi.KittyGraphics([]byte("AAAA"), "a=q", fmt.Sprintf("i=%d", m.ProbeID), "s=1", "v=1", "f=24", "t=d"))
}

func (m *Manager) wrap(value string) string {
	if m.Tmux {
		return ansi.TmuxPassthrough(value)
	}
	return value
}

func (m *Manager) delete(id int) string {
	return m.wrap(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", id), "q=2"))
}

func (m *Manager) Plan(ctx context.Context, requests []Request) ([]Job, string) {
	wanted := make(map[Request]bool)
	var ordered []Request
	for _, req := range requests {
		if len(wanted) >= MaxImages {
			break
		}
		u, err := url.Parse(req.URL)
		local := req.URL == "" && req.Ring != "" && validIcon(req.Icon)
		remote := req.Icon == "" && err == nil && m.validURL(u)
		if (local || remote) && req.Cols > 0 && req.Cols <= 96 && req.Rows > 0 && req.Rows <= 48 && !wanted[req] {
			wanted[req] = true
			ordered = append(ordered, req)
		}
	}
	avatars := make(map[avatarKey]*image.NRGBA)
	for req, e := range m.entries {
		if e.ready && e.avatar != nil {
			avatars[avatarKey{req.URL, req.Ring, req.Icon}] = e.avatar
		}
	}
	var cleanup strings.Builder
	for req, e := range m.entries {
		if !wanted[req] || !m.Enabled {
			e.cancel()
			if e.ready {
				cleanup.WriteString(m.delete(e.id))
			}
			delete(m.entries, req)
		}
	}
	var jobs []Job
	if !m.Enabled {
		return jobs, cleanup.String()
	}
	active := 0
	loading := make(map[avatarKey]bool)
	for req, e := range m.entries {
		if !e.ready && !e.failed {
			active++
			if req.Ring != "" {
				loading[avatarKey{req.URL, req.Ring, req.Icon}] = true
			}
		}
	}
	for _, req := range ordered {
		if active >= cap(m.slots) {
			break
		}
		if _, ok := m.entries[req]; ok {
			continue
		}
		key := avatarKey{req.URL, req.Ring, req.Icon}
		if req.Ring != "" && loading[key] {
			continue
		}
		if m.next >= 0xffffff {
			break
		}
		m.next++
		jobCtx, cancel := context.WithCancel(ctx)
		m.entries[req] = &entry{id: m.next, cancel: cancel}
		jobs = append(jobs, Job{Request: req, ID: m.next, ctx: jobCtx, avatar: avatars[key]})
		active++
		if req.Ring != "" {
			loading[key] = true
		}
	}
	return jobs, cleanup.String()
}

func (m *Manager) Accept(result Result) string {
	e, ok := m.entries[result.Job.Request]
	if !ok || e.id != result.Job.ID || !m.Enabled || e.ready || e.failed {
		return ""
	}
	e.cancel()
	e.failed = result.Err != nil
	e.ready = result.Err == nil
	if e.failed {
		return ""
	}
	e.avatar = result.avatar
	return result.Raw
}

func (m *Manager) Row(req Request, row int) string {
	e, ok := m.entries[req]
	if !m.Enabled || !ok || !e.ready || row < 0 || row >= req.Rows {
		return ""
	}
	var cells strings.Builder
	for col := range req.Cols {
		cells.WriteRune(kitty.Placeholder)
		cells.WriteRune(kitty.Diacritic(row))
		cells.WriteRune(kitty.Diacritic(col))
	}
	id := e.id
	return ansi.Style{}.ForegroundColor(color.RGBA{R: byte((id >> 16) & 255), G: byte((id >> 8) & 255), B: byte(id & 255), A: 255}).Styled(cells.String())
}

func (m *Manager) Clear() string {
	m.Forget()
	_, cleanup := m.Plan(context.Background(), nil)
	return cleanup
}

// Forget drops downloaded images so a later session cannot reuse them.
func (m *Manager) Forget() {
	m.sources.clear()
}

func (m *Manager) Fetch(job Job) Result {
	result := Result{Job: job}
	select {
	case m.slots <- struct{}{}:
	case <-job.ctx.Done():
		result.Err = errors.New("image unavailable")
		return result
	}
	defer func() { <-m.slots }()
	ctx, cancel := context.WithTimeout(job.ctx, 12*time.Second)
	defer cancel()
	picture := job.avatar
	if picture == nil && job.Request.Icon != "" {
		if job.Request.URL != "" || !validIcon(job.Request.Icon) {
			result.Err = errors.New("icon unavailable")
			return result
		}
		picture = notificationIcon(job.Request.Icon, job.Request.Ring)
	}
	if picture == nil {
		var err error
		imageData := m.sources.get(job.Request.URL)
		if imageData == nil {
			if imageData, err = m.download(ctx, job.Request.URL); err != nil {
				result.Err = errors.New("image unavailable")
				return result
			}
		}
		picture, err = prepare(imageData, job.Request)
		if err != nil {
			result.Err = errors.New("image unavailable")
			return result
		}
		m.sources.put(job.Request.URL, imageData)
	}
	if ctx.Err() != nil {
		result.Err = errors.New("image unavailable")
		return result
	}
	encoded := picture
	if job.Request.Ring != "" {
		encoded = frameAvatar(picture, job.Request.Cols, job.Request.Rows)
	}
	var raw bytes.Buffer
	options := &kitty.Options{Action: kitty.TransmitAndPut, Format: kitty.PNG, Transmission: kitty.Direct, ID: job.ID, Quiet: 2, Columns: job.Request.Cols, Rows: job.Request.Rows, VirtualPlacement: true, Chunk: true}
	if m.Tmux {
		options.ChunkFormatter = ansi.TmuxPassthrough
	}
	if err := kitty.EncodeGraphics(&raw, encoded, options); err != nil {
		result.Err = errors.New("image unavailable")
		return result
	}
	result.Raw = raw.String()
	if job.Request.Ring != "" {
		result.avatar = picture
	}
	return result
}

func AvatarURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "avatars.githubusercontent.com" || u.User != nil || u.Fragment != "" {
		return raw
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || query.Has("s") {
		return raw
	}
	query.Set("s", "96")
	u.RawQuery = query.Encode()
	return u.String()
}

func (m *Manager) validURL(u *url.URL) bool {
	if u == nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return m.origin != nil && u.Scheme == "http" && u.Scheme == m.origin.Scheme && u.Host == m.origin.Host && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())
}

func (m *Manager) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("image host unavailable")
	}
	trusted := false
	if m.origin != nil {
		originPort := m.origin.Port()
		if originPort == "" {
			originPort = "443"
			if m.origin.Scheme == "http" {
				originPort = "80"
			}
		}
		trusted = strings.EqualFold(host, m.origin.Hostname()) && port == originPort
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, ControlContext: func(_ context.Context, _ string, address string, _ syscall.RawConn) error {
		if trusted {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return errors.New("image host unavailable")
		}
		ip := net.ParseIP(host)
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return errors.New("image host unavailable")
		}
		return nil
	}}
	return dialer.DialContext(ctx, network, address)
}

func (m *Manager) download(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || !m.validURL(u) {
		return nil, errors.New("image unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	response, err := m.client.Do(req)
	if err != nil {
		return nil, errors.New("image unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	const limit = 10 << 20
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return nil, errors.New("image unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("image unavailable")
	}
	return data, nil
}

func Fit(width, height, maxCols, maxRows int) (int, int) {
	if width <= 0 || height <= 0 || maxCols <= 0 || maxRows <= 0 {
		return 0, 0
	}
	cols := min(maxCols, 96)
	rows := max(1, int(float64(cols)*float64(height)/float64(width)/2+0.5))
	if rows > min(maxRows, 48) {
		rows = min(maxRows, 48)
		cols = max(1, min(cols, int(float64(rows)*2*float64(width)/float64(height)+0.5)))
	}
	return cols, rows
}

func parseRing(value string) color.NRGBA {
	n, _ := strconv.ParseUint(strings.TrimPrefix(value, "#"), 16, 24)
	return color.NRGBA{R: byte((n >> 16) & 255), G: byte((n >> 8) & 255), B: byte(n & 255), A: 255}
}
