package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/subs"
)

// Callback data: m main, s subscription, d devices, dc:<id> confirm unbind, du:<id>
// unbind, c connect, r renew, p:<button> the admin's page, w switch list, u:<user> show
// that subscription, rm confirm removing it from the account, rd:<user> remove it.

// screen renders what the chat sees for a press: text (HTML) and buttons.
func (b *Bot) screen(ctx context.Context, cfg Config, chat int64, data, notice string) (string, *Keyboard) {
	w := wordsFor(cfg.Lang)
	list, u, ok := b.subs(ctx, chat)
	cmd, arg, _ := strings.Cut(data, ":")
	// Buying a new subscription works with or without one.
	if b.canBuyNew(ctx) {
		home := []Button{{Text: w.back, CallbackData: "m"}}
		switch cmd {
		case "b":
			return b.shopList(ctx, w, w.buyTitle, "tn", notice, home)
		case "tn":
			return b.shopTariff(ctx, w, arg, "tn", "pn", shopBack(w, "tn", arg, "b"))
		case "pn":
			id, _, _ := strings.Cut(arg, ":")
			return b.shopInvoice(ctx, w, chat, 0, arg, []Button{{Text: w.back, CallbackData: "tn:" + id}})
		}
	}
	if cmd == "tr" {
		return b.takeTrial(ctx, w, chat)
	}
	if !ok {
		return b.welcome(ctx, cfg, w, chat, notice)
	}
	now := b.d.Now()
	vars := b.vars(ctx, w, u, now)
	back := []Button{{Text: w.back, CallbackData: "m"}}
	withNotice := func(s string) string {
		if notice != "" {
			return html.EscapeString(notice) + "\n\n" + s
		}
		return s
	}
	switch cmd {
	case "t":
		return b.shopTariff(ctx, w, arg, "t", "py", shopBack(w, "t", arg, "r"))
	case "py":
		id, _, _ := strings.Cut(arg, ":")
		return b.shopInvoice(ctx, w, chat, u.ID, arg, []Button{{Text: w.back, CallbackData: "t:" + id}})
	case "s":
		lines := []string{"<b>" + html.EscapeString(fmt.Sprintf(w.subTitle, u.Name)) + "</b>", html.EscapeString(vars["state"]), "",
			"📅 " + html.EscapeString(vars["term"]), "📦 " + html.EscapeString(vars["traffic"])}
		for _, p := range b.poolLines(ctx, w, u.ID) {
			lines = append(lines, "📦 "+html.EscapeString(p))
		}
		if r := vars["reset"]; r != "" {
			lines = append(lines, html.EscapeString(fmt.Sprintf(w.resets, r)))
		}
		lines = append(lines, "📱 "+html.EscapeString(vars["devices"]))
		rows := [][]Button{}
		if offers := b.packageOffers(ctx, u.ID); len(offers) > 0 {
			rows = append(rows, []Button{{Text: w.buyTraffic, CallbackData: "x"}})
		}
		rows = append(rows, []Button{{Text: w.removeSub, CallbackData: "rm"}})
		return withNotice(strings.Join(lines, "\n")), &Keyboard{append(rows, back)}
	case "rm":
		// The id goes with the button: a tap after switching subscriptions removes the
		// one that was asked about, not the one shown now.
		return withNotice(html.EscapeString(fmt.Sprintf(w.confirmRemove, u.Name))), &Keyboard{[][]Button{
			{{Text: w.yesRemove, CallbackData: "rd:" + strconv.FormatInt(u.ID, 10)}, {Text: w.cancel, CallbackData: "s"}}}}
	case "x", "xk", "xp":
		return b.trafficShop(ctx, w, chat, u, cmd, arg, notice)
	case "d", "dc":
		id, _ := strconv.ParseInt(arg, 10, 64)
		return b.devices(ctx, w, u, cmd, id, notice, now)
	case "c":
		text := "<b>" + w.connectTitle + "</b>\n\n" + fmt.Sprintf(w.connectText, html.EscapeString(b.subURL(ctx, u)))
		rows := [][]Button{}
		if btn, ok := b.pageButton(ctx, cfg, w, w.openPage); ok {
			rows = append(rows, []Button{btn})
		}
		rows = append(rows, []Button{{Text: w.freeturnTitle, CallbackData: "ft"}})
		return withNotice(text), &Keyboard{append(rows, back)}
	case "ft":
		return b.freeTurnScreen(ctx, cfg, w, u, notice)
	case "r":
		if offers, _ := b.offers(ctx); len(offers) > 0 {
			text, kb := b.shopList(ctx, w, fmt.Sprintf(w.renewTitle, u.Name), "t", notice, nil)
			if sup := b.supportURL(ctx); sup != "" {
				kb.InlineKeyboard = append(kb.InlineKeyboard, []Button{{Text: labelOf(cfg, "support", w.support), URL: sup}})
			}
			kb.InlineKeyboard = append(kb.InlineKeyboard, back)
			return text, kb
		}
		rows := [][]Button{}
		if sup := b.supportURL(ctx); sup != "" {
			rows = append(rows, []Button{{Text: labelOf(cfg, "support", w.support), URL: sup}})
		}
		return withNotice(render(pick(cfg.Texts.Renew, w.renew), vars)), &Keyboard{append(rows, back)}
	case "p":
		for _, btn := range cfg.Buttons {
			if btn.Action == "page" && btn.ID == arg {
				return withNotice(render(btn.Text, vars)), &Keyboard{[][]Button{back}}
			}
		}
	case "w":
		rows := [][]Button{}
		for _, s := range list {
			mark := ""
			if s.ID == u.ID {
				mark = "✓ "
			}
			rows = append(rows, []Button{{Text: mark + s.Name, CallbackData: "u:" + strconv.FormatInt(s.ID, 10)}})
		}
		return w.switchTitle, &Keyboard{append(rows, back)}
	}
	return withNotice(render(pick(cfg.Texts.Main, w.main), vars)), b.menu(ctx, cfg, w, len(list))
}

func (b *Bot) welcome(ctx context.Context, cfg Config, w *words, chat int64, notice string) (string, *Keyboard) {
	brand := b.brand(ctx)
	text := render(pick(cfg.Texts.Welcome, w.welcome), map[string]string{"brand": brand})
	if notice != "" {
		text = html.EscapeString(notice) + "\n\n" + text
	}
	var rows [][]Button
	if b.d.Billing != nil && b.d.Billing.TrialOpen(ctx, chat) {
		rows = append(rows, []Button{{Text: w.trial, CallbackData: "tr"}})
	}
	if b.canBuyNew(ctx) {
		rows = append(rows, []Button{{Text: w.buy, CallbackData: "b"}})
	}
	if sup := b.supportURL(ctx); sup != "" {
		rows = append(rows, []Button{{Text: labelOf(cfg, "support", w.support), URL: sup}})
	}
	if rows == nil {
		return text, nil
	}
	return text, &Keyboard{rows}
}

// menu is the admin's main menu as buttons.
func (b *Bot) menu(ctx context.Context, cfg Config, w *words, subs int) *Keyboard {
	var rows [][]Button
	for _, mb := range cfg.Buttons {
		if !mb.On {
			continue
		}
		var btn Button
		switch mb.Action {
		case "sub":
			btn = Button{Text: mb.Label, CallbackData: "s"}
		case "devices":
			btn = Button{Text: mb.Label, CallbackData: "d"}
		case "connect":
			btn = Button{Text: mb.Label, CallbackData: "c"}
		case "renew":
			btn = Button{Text: mb.Label, CallbackData: "r"}
		case "support":
			sup := b.supportURL(ctx)
			if sup == "" {
				continue
			}
			btn = Button{Text: mb.Label, URL: sup}
		case "app":
			var ok bool
			if btn, ok = b.pageButton(ctx, cfg, w, mb.Label); !ok {
				continue
			}
		case "url":
			btn = Button{Text: mb.Label, URL: mb.URL}
		case "page":
			btn = Button{Text: mb.Label, CallbackData: "p:" + mb.ID}
		default:
			continue
		}
		if mb.Row && len(rows) > 0 && len(rows[len(rows)-1]) < 3 {
			rows[len(rows)-1] = append(rows[len(rows)-1], btn)
		} else {
			rows = append(rows, []Button{btn})
		}
	}
	if url := b.miniAppURL(ctx, cfg); url != "" {
		rows = append(rows, []Button{{Text: w.promo, WebApp: &WebApp{URL: url + "#promocodes"}}})
	}
	if subs > 1 {
		rows = append(rows, []Button{{Text: fmt.Sprintf("%s (%d)", w.subscriptions, subs), CallbackData: "w"}})
	}
	return &Keyboard{InlineKeyboard: rows}
}

// pageButton opens the subscription page: in the Mini App when Telegram can load it,
// else in the browser.
func (b *Bot) pageButton(ctx context.Context, cfg Config, w *words, label string) (Button, bool) {
	if url := b.miniAppURL(ctx, cfg); url != "" {
		return Button{Text: label, WebApp: &WebApp{URL: url}}, true
	}
	return Button{}, false
}

func (b *Bot) devices(ctx context.Context, w *words, u db.User, cmd string, id int64, notice string, now time.Time) (string, *Keyboard) {
	back := []Button{{Text: w.back, CallbackData: "m"}}
	binding, _ := b.d.Settings.On(ctx, settings.DeviceBinding)
	head := "<b>" + w.devicesTitle + "</b> · " + html.EscapeString(b.vars(ctx, w, u, now)["devices"])
	if !binding {
		limit := "∞"
		if u.DeviceLimit.Valid {
			limit = strconv.FormatInt(u.DeviceLimit.Int64, 10)
		}
		return head + "\n\n" + html.EscapeString(fmt.Sprintf(w.devicesOff, limit)), &Keyboard{[][]Button{back}}
	}
	devs, err := b.d.Store.Q.ListBoundDevices(ctx, u.ID)
	if err != nil {
		devs = nil
	}
	name := func(d db.BoundDevice) string { return deviceName(w, d) }
	if cmd == "dc" {
		for _, d := range devs {
			if d.ID == id {
				return html.EscapeString(fmt.Sprintf(w.confirmUnbind, name(d))), &Keyboard{[][]Button{
					{{Text: w.yesUnbind, CallbackData: "du:" + strconv.FormatInt(id, 10)}, {Text: w.cancel, CallbackData: "d"}}}}
			}
		}
	}
	lines := []string{head, ""}
	if notice != "" {
		lines = append([]string{html.EscapeString(notice), ""}, lines...)
	}
	rows := [][]Button{}
	if len(devs) == 0 {
		lines = append(lines, html.EscapeString(w.devicesNone))
	}
	for i, d := range devs {
		meta := []string{}
		if d.Hwid != "" && d.Model != "" && d.Os != "" {
			meta = append(meta, strings.TrimSpace(d.Os+" "+d.OsVersion))
		}
		if app, _, _ := strings.Cut(strings.TrimSpace(d.App), " "); app != "" {
			meta = append(meta, strings.Replace(app, "/", " ", 1))
		}
		meta = append(meta, w.ago(time.Unix(d.LastSeen, 0), now))
		lines = append(lines, fmt.Sprintf("%d. %s — %s", i+1, html.EscapeString(name(d)), html.EscapeString(strings.Join(meta, " · "))))
		rows = append(rows, []Button{{Text: "❌ " + name(d), CallbackData: "dc:" + strconv.FormatInt(d.ID, 10)}})
	}
	if len(devs) > 0 {
		lines = append(lines, "", html.EscapeString(w.devicesNote))
	}
	return strings.Join(lines, "\n"), &Keyboard{append(rows, back)}
}

func deviceName(w *words, d db.BoundDevice) string {
	switch {
	case d.Hwid == "":
		return w.sharedPlace
	case d.Model != "":
		return d.Model
	case d.Os != "":
		return strings.TrimSpace(d.Os + " " + d.OsVersion)
	}
	return w.device
}

// act does what a tap changes — unbinding a device, showing another subscription — and
// says which screen to draw after it, with a line on top. A tap's effect never waits in
// the outbox: only its screen does, and a later tap may replace that.
func (b *Bot) act(ctx context.Context, chat int64, data string) (screen, notice string) {
	cmd, arg, _ := strings.Cut(data, ":")
	id, _ := strconv.ParseInt(arg, 10, 64)
	switch cmd {
	case "ftc":
		w := wordsFor(b.Config(ctx).Lang)
		_, u, ok := b.subs(ctx, chat)
		if !ok {
			return "m", ""
		}
		nodeFTs, err := b.d.Store.Q.ListNodeFreeTurn(ctx)
		if err != nil || len(nodeFTs) == 0 {
			msg := "FreeTurn недоступен"
			if b.Config(ctx).Lang == "en" {
				msg = "FreeTurn is unavailable"
			}
			return "ft", msg
		}
		_, conf, err := b.userFreeTurnLinkAndConf(ctx, u, nodeFTs)
		if err != nil || conf == "" {
			msg := "Не удалось сформировать .conf файл"
			if b.Config(ctx).Lang == "en" {
				msg = "Failed to generate .conf file"
			}
			return "ft", msg
		}
		caption := fmt.Sprintf(w.freeturnDocCaption, u.Name)
		fileName := fmt.Sprintf("freeturn-%s.conf", u.Name)
		if out := b.out.Load(); out != nil {
			out.Reply(chat, "doc:"+fileName, 1, func(ctx context.Context, c *Client) error {
				reader := strings.NewReader(conf)
				_, err := c.SendDocument(ctx, chat, fileName, reader, int64(len(conf)), caption)
				return err
			})
		}
		return "ft", w.freeturnDocSent
	case "du":
		w := wordsFor(b.Config(ctx).Lang)
		_, u, ok := b.subs(ctx, chat)
		if !ok {
			return "m", ""
		}
		devs, _ := b.d.Store.Q.ListBoundDevices(ctx, u.ID)
		for _, d := range devs {
			if d.ID != id {
				continue
			}
			err := b.d.Devices.Unbind(ctx, u.ID, id, true)
			switch {
			case err == nil:
				return "d", fmt.Sprintf(w.unbound, deviceName(w, d))
			case errors.Is(err, domain.ErrUnbindCooldown):
				fresh, _ := b.d.Store.Q.GetUser(ctx, u.ID)
				return "d", fmt.Sprintf(w.wait, b.when(w, domain.NextUnbind(fresh, b.d.Now())))
			}
		}
		return "d", ""
	case "u":
		list, _, _ := b.subs(ctx, chat)
		for _, s := range list {
			if s.ID == id {
				_ = b.d.Store.Q.SetTgCurrent(ctx, db.SetTgCurrentParams{Current: id, TgID: chat})
			}
		}
		return "m", ""
	case "rd":
		// Only a subscription of this account, and only its link: the subscription itself
		// stays as it is. The next one in the list is shown after it.
		w := wordsFor(b.Config(ctx).Lang)
		list, _, _ := b.subs(ctx, chat)
		for _, s := range list {
			if s.ID != id {
				continue
			}
			if err := b.d.Store.Q.UnlinkTg(ctx, id); err != nil {
				b.d.Log.Warn("telegram: remove a subscription", "user", id, "err", err)
				return "s", ""
			}
			b.d.Log.Info("telegram: a subscription removed from its account", "user", id)
			return "m", fmt.Sprintf(w.removed, s.Name)
		}
		return "m", ""
	}
	return data, ""
}

func (b *Bot) when(w *words, t time.Time) string {
	if t.IsZero() {
		return w.justNow
	}
	return w.shortDate(t) + " " + t.Format("15:04") + " UTC"
}

// vars are the {variables} of the admin's texts for one subscription.
func (b *Bot) vars(ctx context.Context, w *words, u db.User, now time.Time) map[string]string {
	grants, err := domain.UserGrantsLeft(ctx, b.d.Store.Q, u.ID, now)
	if err != nil {
		b.d.Log.Warn("telegram: traffic packages", "err", err)
	}
	extra := grants.Main(u.ID)
	state := domain.State(u, extra, now)
	v := map[string]string{"name": u.Name, "brand": b.brand(ctx), "until": w.forever, "days": "—", "term": w.forever, "reset": ""}
	switch state {
	case domain.StateActive:
		v["state"] = w.stateActive
	case domain.StateExpiring:
		v["state"] = w.stateExpiring
	case domain.StateLimited:
		v["state"] = w.stateLimited
	case domain.StateExpired:
		v["state"] = w.stateExpired
	default:
		v["state"] = w.stateOff
	}
	if u.ExpiresAt.Valid {
		exp := time.Unix(u.ExpiresAt.Int64, 0).UTC()
		left := int((exp.Sub(now) + 24*time.Hour - time.Second) / (24 * time.Hour))
		v["until"] = w.date(exp)
		v["days"] = w.days(max(0, left))
		v["term"] = fmt.Sprintf(w.termUntil, v["until"], v["days"])
	}
	used := u.UsedUp + u.UsedDown
	v["used"] = w.bytes(used)
	if u.TrafficLimit.Valid {
		v["limit"] = w.bytes(u.TrafficLimit.Int64)
		v["left"] = w.bytes(domain.TrafficLeft(u.TrafficLimit, used, extra))
		v["traffic"] = fmt.Sprintf(w.trafficOf, v["used"], w.withPackages(u.TrafficLimit.Int64, extra))
	} else {
		v["limit"], v["left"] = w.noLimit, w.noLimit
		v["traffic"] = fmt.Sprintf(w.trafficNoLimit, v["used"])
	}
	if t, ok := domain.NextReset(u, now); ok {
		v["reset"] = w.shortDate(t)
	}
	n, _ := b.d.Store.Q.CountBoundDevices(ctx, u.ID)
	v["devices"] = strconv.FormatInt(n, 10)
	if u.DeviceLimit.Valid {
		v["devices"] = fmt.Sprintf(w.trafficOf, v["devices"], strconv.FormatInt(u.DeviceLimit.Int64, 10))
	}
	return v
}

func (b *Bot) brand(ctx context.Context) string {
	if s, _ := b.d.Settings.String(ctx, settings.KeyBrand); s != "" {
		return s
	}
	return "VPN"
}

// supportURL is the panel's support link when Telegram can open it.
func (b *Bot) supportURL(ctx context.Context) string {
	s, _ := b.d.Settings.String(ctx, settings.KeySupportURL)
	if safeURL(s) {
		return s
	}
	return ""
}

func (b *Bot) subURL(ctx context.Context, u db.User) string {
	if base := b.d.SubBase(ctx); base != "" {
		return base + "/" + u.SubToken
	}
	return ""
}

// poolLines: one line per traffic pool with a limit — "WL: 30 GB of 100 GB + packages 50 GB".
func (b *Bot) poolLines(ctx context.Context, w *words, userID int64) []string {
	rows, err := b.d.Store.Q.ListUserPools(ctx, userID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	pools, err := b.d.Store.Q.ListTrafficPools(ctx)
	if err != nil {
		return nil
	}
	grants, err := domain.UserGrantsLeft(ctx, b.d.Store.Q, userID, b.d.Now())
	if err != nil {
		return nil
	}
	names := map[int64]string{}
	for _, p := range pools {
		names[p.ID] = p.Name
	}
	var out []string
	for _, r := range rows {
		if !r.TrafficLimit.Valid {
			continue
		}
		extra := grants.Pool(userID, r.PoolID)
		line := names[r.PoolID] + ": " + fmt.Sprintf(w.trafficOf, w.bytes(r.UsedUp+r.UsedDown), w.withPackages(r.TrafficLimit.Int64, extra))
		if domain.PoolExhausted(r, extra) {
			line += " — " + w.poolOut
		}
		out = append(out, line)
	}
	return out
}

func (b *Bot) freeTurnScreen(ctx context.Context, cfg Config, w *words, u db.User, notice string) (string, *Keyboard) {
	back := []Button{{Text: w.back, CallbackData: "c"}}
	nodeFTs, err := b.d.Store.Q.ListNodeFreeTurn(ctx)
	if err != nil || len(nodeFTs) == 0 {
		msg := "FreeTurn временно недоступен или ещё не настроен на сервере."
		if cfg.Lang == "en" {
			msg = "FreeTurn is temporarily unavailable or not yet configured on the server."
		}
		return msg, &Keyboard{[][]Button{back}}
	}

	directLink, _, err := b.userFreeTurnLinkAndConf(ctx, u, nodeFTs)
	if err != nil {
		b.d.Log.Warn("telegram: get freeturn config", "user", u.ID, "err", err)
		msg := "Не удалось получить конфигурацию FreeTurn. Попробуйте позже."
		if cfg.Lang == "en" {
			msg = "Failed to load FreeTurn configuration. Try again later."
		}
		return msg, &Keyboard{[][]Button{back}}
	}

	subURL := b.subURL(ctx, u)
	ftSubURL := ""
	if subURL != "" {
		ftSubURL = subURL + "?format=freeturn"
	}

	text := fmt.Sprintf(w.freeturnConnect, html.EscapeString(ftSubURL), html.EscapeString(directLink))
	if notice != "" {
		text = html.EscapeString(notice) + "\n\n" + text
	}

	rows := [][]Button{
		{{Text: "📥 Скачать WireGuard .conf", CallbackData: "ftc"}},
		{{Text: "📱 Скачать FreeTurn APK", URL: "https://github.com/hackdiaz-dev/turn-proxy-android/releases"}},
		back,
	}
	if cfg.Lang == "en" {
		rows[0] = []Button{{Text: "📥 Download WireGuard .conf", CallbackData: "ftc"}}
		rows[1] = []Button{{Text: "📱 Download FreeTurn APK", URL: "https://github.com/hackdiaz-dev/turn-proxy-android/releases"}}
	}
	return text, &Keyboard{rows}
}

func (b *Bot) userFreeTurnLinkAndConf(ctx context.Context, u db.User, nodeFTs []db.NodeFreeturn) (string, string, error) {
	if len(nodeFTs) == 0 {
		var err error
		nodeFTs, err = b.d.Store.Q.ListNodeFreeTurn(ctx)
		if err != nil || len(nodeFTs) == 0 {
			return "", "", errors.New("no freeturn nodes")
		}
	}

	var nft db.NodeFreeturn
	var n db.Node
	found := false
	for _, candidate := range nodeFTs {
		node, err := b.d.Store.Q.GetNode(ctx, candidate.NodeID)
		if err == nil && node.Enabled != 0 {
			nft = candidate
			n = node
			found = true
			break
		}
	}
	if !found {
		return "", "", errors.New("no enabled freeturn nodes")
	}

	host := n.Domain
	if host == "" {
		host = n.PublicHost
	}
	if host == "" && n.Address != "" {
		if h, _, err := net.SplitHostPort(n.Address); err == nil {
			host = h
		} else {
			host = n.Address
		}
	}
	if host == "" {
		if dom, _ := b.d.Settings.String(ctx, settings.KeyDomain); dom != "" {
			host = dom
		} else if pub, _ := b.d.Settings.String(ctx, settings.KeyPublicHost); pub != "" {
			host = pub
		} else if b.d.SubBase != nil {
			if u, err := url.Parse(b.d.SubBase(ctx)); err == nil {
				host = u.Hostname()
			}
		}
	}
	if host == "" {
		var dom string
		if raw, err := b.d.Store.Q.GetSetting(ctx, "domain"); err == nil {
			_ = json.Unmarshal([]byte(raw), &dom)
		}
		if dom != "" {
			host = dom
		} else {
			var pub string
			if raw, err := b.d.Store.Q.GetSetting(ctx, "public_host"); err == nil {
				_ = json.Unmarshal([]byte(raw), &pub)
			}
			if pub != "" {
				host = pub
			}
		}
	}
	name := n.Name
	if name == "" {
		brand, _ := b.d.Settings.String(ctx, settings.KeyBrand)
		if brand != "" {
			name = brand
		} else {
			name = "FreeTurn"
		}
	}
	peer := net.JoinHostPort(host, strconv.FormatInt(nft.ListenPort, 10))
	node := subs.FreeTurnNode{
		Name:       name,
		Peer:       peer,
		ObfProfile: nft.ObfProfile,
		ObfKey:     nft.ObfKey,
		WgPubKey:   nft.WgPubkey,
	}

	fts, err := b.d.Store.Q.GetUserFreeTurn(ctx, u.ID)
	if err != nil {
		return "", "", err
	}

	var client subs.FreeTurnClient
	if len(fts) > 0 {
		client = subs.FreeTurnClient{
			ClientID:   fts[0].ClientID,
			WgPrivKey:  fts[0].WgPrivkey,
			WgIP:       fts[0].WgIp,
			ClientName: u.Name,
		}
	} else {
		cid, err := subs.GenerateClientID()
		if err != nil {
			return "", "", err
		}
		privKey, pubKey, err := subs.GenerateWireGuardKeypair()
		if err != nil {
			return "", "", err
		}
		usedIPs, err := b.d.Store.Q.ListUsedFreeTurnIPs(ctx)
		if err != nil {
			return "", "", err
		}
		netCIDR := nft.WgNetwork
		if netCIDR == "" {
			netCIDR = "10.13.13.0/24"
		}
		wgIP, err := subs.NextFreeIP(usedIPs, netCIDR)
		if err != nil {
			return "", "", err
		}
		slotID := int64(0)
		if u.SlotID.Valid {
			slotID = u.SlotID.Int64
		} else if slots, err := b.d.Store.Q.ListSlots(ctx); err == nil && len(slots) > 0 {
			slotID = slots[0].ID
		}
		if slotID == 0 {
			slotID = 1
		}
		newFT, err := b.d.Store.Q.CreateUserFreeTurn(ctx, db.CreateUserFreeTurnParams{
			UserID:     u.ID,
			SlotID:     slotID,
			ClientID:   cid,
			WgPubkey:   pubKey,
			WgPrivkey:  privKey,
			WgIp:       wgIP,
			DeviceName: "FreeTurn Client",
			CreatedAt:  b.d.Now().Unix(),
		})
		if err != nil {
			return "", "", err
		}
		client = subs.FreeTurnClient{
			ClientID:   newFT.ClientID,
			WgPrivKey:  newFT.WgPrivkey,
			WgIP:       newFT.WgIp,
			ClientName: u.Name,
		}
	}

	link := subs.BuildFreeTurnLink(node, client)
	conf := subs.BuildFreeTurnWireGuardConfig(node, client)
	return link, conf, nil
}
