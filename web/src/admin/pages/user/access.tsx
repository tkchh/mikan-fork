import { Copy, ExternalLink, RefreshCw, Send } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, unwrap, type User } from "../../../api/client";
import { useInbounds, userActions, useUserMutation } from "../../../api/hooks";
import { Confirm } from "../../../components/overlay";
import { useToast } from "../../../components/toast";
import { Button, QR } from "../../../components/ui";
import { Switch } from "../../../components/switch";
import { t } from "../../../i18n";
import { useCopy } from "../../../lib/copy";
import { useDraft } from "../../../lib/draft";
import { safeHref } from "../../../lib/url";
import { Section } from "./section";

export function SubscriptionSection({ u, onReissue }: { u: User; onReissue: () => void }) {
  const copyText = useCopy();
  const copy = () => copyText(u.sub_url, t("common.linkCopied"));
  if (!u.sub_url) {
    return (
      <Section title={t("userDrawer.subscription")}>
        <p className="text-[13px] text-[var(--ink-500)]">{t("userDrawer.noHost")}</p>
        {u.legacy ? <LegacyLink legacy={u.legacy} /> : null}
      </Section>
    );
  }
  return (
    <Section title={t("userDrawer.subscription")}>
      <div className="link-field">
        <span className="mono" title={u.sub_url}>
          {u.sub_url}
        </span>
        <button type="button" className="icon-btn" onClick={copy} aria-label={t("common.copyLink")}>
          <Copy size={18} />
        </button>
      </div>
      <div className="mt-4 grid grid-cols-1 items-start gap-4 sm:grid-cols-[136px_minmax(0,1fr)]">
        <QR value={u.sub_url} />
        <div className="flex flex-col items-start gap-2">
          <p className="mb-1 text-[13px] text-[var(--ink-500)]">{t("userDrawer.sendHint")}</p>
          <a className="btn btn-glass btn-sm" href={safeHref(u.sub_url)} target="_blank" rel="noreferrer noopener">
            {t("userDrawer.subPage")} <ExternalLink size={14} aria-hidden />
          </a>
          <Button size="sm" variant="danger" onClick={onReissue}>
            <RefreshCw size={16} aria-hidden /> {t("userDrawer.reissue")}
          </Button>
        </div>
      </div>
      <HappLink id={u.id} token={u.sub_url} />
      {u.legacy ? <LegacyLink legacy={u.legacy} /> : null}
    </Section>
  );
}

/** Happ's crypt link, when the panel makes them (Settings → Subscription → Happ): what to
 * send instead of the address when the address must stay hidden. Nothing while it loads
 * or when the crypt link is off; token refetches it after a reissue. */
function HappLink({ id, token }: { id: number; token: string }) {
  const copyText = useCopy();
  const q = useQuery({
    queryKey: ["happ-link", id, token],
    queryFn: ({ signal }) => unwrap(api.GET("/api/v1/users/{id}/happ-link", { params: { path: { id } }, signal })),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const link = q.data?.link;
  if (!link) return null;
  return (
    <div className="mt-4">
      <div className="mb-1 text-[13px] font-medium">{t("settings.happLink")}</div>
      <div className="link-field">
        <span className="mono" title={link}>
          {link}
        </span>
        <button type="button" className="icon-btn" onClick={() => copyText(link, t("common.linkCopied"))} aria-label={t("common.copyLink")}>
          <Copy size={18} />
        </button>
      </div>
      <p className="mt-1 text-xs text-[var(--ink-500)]">{t("settings.happLinkHint")}</p>
    </div>
  );
}

type Legacy = NonNullable<User["legacy"]>;

const SOURCES = { marzban: "Marzban", pasarguard: "PasarGuard", remnawave: "Remnawave" } as Record<string, string>;

/** The link the user had in the panel they came from. Only Remnawave's can be shown whole:
 * Marzban and PasarGuard sign a new token for every request, so the panel keeps none. */
function LegacyLink({ legacy }: { legacy: Legacy }) {
  const copyText = useCopy();
  const source = SOURCES[legacy.source] ?? legacy.source;
  const hint = !legacy.active
    ? t("userDrawer.legacyOff")
    : legacy.url
      ? t("userDrawer.legacyUrl")
      : legacy.source === "remnawave"
        ? t("userDrawer.legacyNoHost")
        : t("userDrawer.legacySigned");
  return (
    <div className="mt-5 border-t border-[var(--hairline)] pt-4">
      <div className="mb-2 text-[13px] font-medium text-[var(--ink-700)]">{t("userDrawer.legacyTitle", { source })}</div>
      {legacy.url ? (
        <div className="link-field mb-2">
          <span className="mono" title={legacy.url}>
            {legacy.url}
          </span>
          <button type="button" className="icon-btn" onClick={() => copyText(legacy.url!, t("common.linkCopied"))} aria-label={t("common.copyLink")}>
            <Copy size={18} />
          </button>
        </div>
      ) : null}
      <p className="text-xs text-[var(--ink-500)]">{hint}</p>
    </div>
  );
}

export function TelegramSection({ u }: { u: User }) {
  const toast = useToast();
  const unlink = useUserMutation((id: number) => unwrap(api.DELETE("/api/v1/users/{id}/telegram", { params: { path: { id } } })));
  const [confirm, setConfirm] = useState(false);
  const tg = u.telegram;
  return (
    <Section title={t("userDrawer.telegram")}>
      {tg ? (
        <div className="panel-soft flex items-center gap-3 p-3">
          <span className="grid h-9 w-9 place-items-center rounded-[10px] bg-[var(--hover)] text-[var(--ink-600)]" aria-hidden>
            <Send size={18} />
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-[13px] font-medium">{tg.username ? `@${tg.username}` : tg.name || tg.id}</div>
            {tg.username && tg.name ? <div className="truncate text-xs text-[var(--ink-500)]">{tg.name}</div> : null}
          </div>
          <Button size="sm" variant="danger" onClick={() => setConfirm(true)}>
            {t("userDrawer.telegramUnlink")}
          </Button>
        </div>
      ) : (
        <p className="text-[13px] text-[var(--ink-500)]">{t("userDrawer.telegramNone")}</p>
      )}
      <Confirm
        open={confirm}
        onOpenChange={setConfirm}
        title={t("userDrawer.telegramUnlinkTitle")}
        text={t("userDrawer.telegramUnlinkText")}
        confirm={t("userDrawer.telegramUnlink")}
        danger
        loading={unlink.isPending}
        onConfirm={() =>
          unlink.mutate(u.id, {
            onSuccess: () => {
              setConfirm(false);
              toast.ok(t("userDrawer.telegramUnlinked"));
            },
            onError: (e) => toast.error(errorText(e)),
          })
        }
      />
    </Section>
  );
}

export function ProtocolsSection({ u }: { u: User }) {
  const inbounds = useInbounds();
  const update = useUserMutation(userActions.update);
  const toast = useToast();
  const all = (inbounds.data ?? []).filter((i) => i.enabled);
  const allowed = new Set(u.inbounds.length ? u.inbounds : all.map((i) => i.id));
  const toggle = (id: number, on: boolean) => {
    const next = new Set(allowed);
    if (on) next.add(id);
    else next.delete(id);
    if (next.size === 0) {
      toast.error(t("userDrawer.needOneProtocol"));
      return;
    }
    const ids = next.size === all.length ? [] : [...next];
    update.mutate({ id: u.id, body: { inbounds: ids } }, { onError: (e) => toast.error(errorText(e)) });
  };
  return (
    <Section title={t("userDrawer.protocols")}>
      {all.map((i) => (
        <div key={i.id} className="flex items-center justify-between gap-3 py-2">
          <div>
            <div className="text-[13px] font-medium">{i.sub_name}</div>
            <div className="text-xs text-[var(--ink-500)]">
              {i.title} · {i.port}/{i.network}
            </div>
          </div>
          <Switch checked={allowed.has(i.id)} onChange={(v) => toggle(i.id, v)} label={i.sub_name} disabled={update.isPending} />
        </div>
      ))}
    </Section>
  );
}

export function NoteSection({ u }: { u: User }) {
  // The drawer polls the user every few seconds: a note being typed is not replaced.
  const { draft: note, setDraft: setNote, dirty } = useDraft(u.note);
  const update = useUserMutation(userActions.update);
  const toast = useToast();
  return (
    <Section title={t("userDrawer.note")} aside={update.isPending ? t("userDrawer.saving") : undefined}>
      <textarea
        className="input"
        aria-label={t("userDrawer.noteLabel")}
        placeholder={t("userDrawer.notePlaceholder")}
        maxLength={2000}
        value={note}
        onChange={(e) => setNote(e.target.value)}
        onBlur={() => {
          if (dirty) update.mutate({ id: u.id, body: { note } }, { onSuccess: () => toast.ok(t("userDrawer.noteSaved")), onError: (e) => toast.error(errorText(e)) });
        }}
      />
    </Section>
  );
}

export function FreeTurnSection({ u }: { u: User }) {
  const toast = useToast();
  const copyText = useCopy();
  const [confirmReset, setConfirmReset] = useState(false);
  const resetMutation = useUserMutation((id: number) =>
    unwrap(api.POST("/api/v1/users/{id}/freeturn/reset" as any, { params: { path: { id } } }))
  );

  const ft = u.freeturn;
  if (!ft) {
    return (
      <Section title={t("userDrawer.freeturnTitle")}>
        <div className="space-y-3">
          <p className="text-xs text-[var(--ink-500)] leading-relaxed">
            {t("userDrawer.freeturnNotGenerated")}
          </p>
          <div className="pt-1">
            <Button
              size="sm"
              variant="glass"
              loading={resetMutation.isPending}
              onClick={() =>
                resetMutation.mutate(u.id, {
                  onSuccess: () => toast.ok(t("userDrawer.freeturnResetDone")),
                  onError: (e) => toast.error(errorText(e)),
                })
              }
            >
              <RefreshCw size={16} aria-hidden /> {t("userDrawer.freeturnGenerate")}
            </Button>
          </div>
        </div>
      </Section>
    );
  }

  return (
    <Section title={t("userDrawer.freeturnTitle")}>
      <div className="space-y-3">
        <div>
          <div className="text-xs text-[var(--ink-500)] mb-1">WireGuard IP</div>
          <div className="link-field">
            <span className="mono">{ft.wg_ip}</span>
            <button type="button" className="icon-btn" onClick={() => copyText(ft.wg_ip, t("common.linkCopied"))} aria-label={t("common.copyLink")}>
              <Copy size={16} />
            </button>
          </div>
        </div>
        <div>
          <div className="text-xs text-[var(--ink-500)] mb-1">Client ID (free-turn-proxy)</div>
          <div className="link-field">
            <span className="mono">{ft.client_id}</span>
            <button type="button" className="icon-btn" onClick={() => copyText(ft.client_id, t("common.linkCopied"))} aria-label={t("common.copyLink")}>
              <Copy size={16} />
            </button>
          </div>
        </div>
        <div>
          <div className="text-xs text-[var(--ink-500)] mb-1">WireGuard Public Key</div>
          <div className="link-field">
            <span className="mono">{ft.wg_pubkey}</span>
            <button type="button" className="icon-btn" onClick={() => copyText(ft.wg_pubkey, t("common.linkCopied"))} aria-label={t("common.copyLink")}>
              <Copy size={16} />
            </button>
          </div>
        </div>
        <p className="text-xs text-[var(--ink-500)] leading-relaxed pt-1">
          {t("userDrawer.freeturnDeviceLimitHint")}
        </p>
        <div className="pt-1">
          <Button size="sm" variant="danger" onClick={() => setConfirmReset(true)}>
            <RefreshCw size={16} aria-hidden /> {t("userDrawer.freeturnReset")}
          </Button>
        </div>
      </div>
      <Confirm
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={t("userDrawer.freeturnResetConfirm")}
        text={t("userDrawer.freeturnResetText")}
        confirm={t("userDrawer.freeturnReset")}
        danger
        loading={resetMutation.isPending}
        onConfirm={() =>
          resetMutation.mutate(u.id, {
            onSuccess: () => {
              setConfirmReset(false);
              toast.ok(t("userDrawer.freeturnResetDone"));
            },
            onError: (e) => toast.error(errorText(e)),
          })
        }
      />
    </Section>
  );
}
