import { LabI18nProvider } from "./lab-i18n";
import { loadLocale, LOCALE_STORAGE_KEY, type LabLocale } from "./locale";
import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import {
  ArrowDownToLine,
  Check,
  ChevronRight,
  Code2,
  Columns2,
  Copy,
  LayoutGrid,
  ListTodo,
  Moon,
  MousePointer2,
  PanelLeft,
  Redo2,
  Save,
  Sun,
  Trash2,
  Undo2,
  X,
} from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { MulticaIcon } from "@multica/ui/components/common/multica-icon";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  type ButtonScale,
  changeCount,
  editHistory,
  emptyDraft,
  exportCss,
  tokenValue,
  updateToken,
  type Draft,
  type Theme,
} from "./tokens";
import { Inspector } from "./inspector";
import { scenes, type PreviewSettings, type Scene } from "./protocol";
import {
  encodeSession,
  loadSession,
  STORAGE_KEY,
  type SavedDesign,
} from "./storage";

function PreviewFrame({
  draft,
  theme,
  scene,
  buttonScale,
  original = false,
}: {
  draft: Draft;
  theme: Theme;
  scene: Scene;
  buttonScale: ButtonScale;
  original?: boolean;
}) {
  const { t, i18n } = useTranslation("uiLab");
  const locale: LabLocale = i18n.language === "zh-Hans" ? "zh" : "en";
  const ref = useRef<HTMLIFrameElement>(null);
  const send = useCallback(() => {
    ref.current?.contentWindow?.postMessage(
      {
        type: "multica-ui-lab:preview",
        draft,
        theme,
        scene,
        buttonScale,
        locale,
      } satisfies PreviewSettings,
      location.origin,
    );
  }, [draft, theme, scene, buttonScale, locale]);
  useEffect(() => {
    const onReady = (event: MessageEvent<{ type?: string }>) => {
      if (
        event.origin === location.origin &&
        event.source === ref.current?.contentWindow &&
        event.data?.type === "multica-ui-lab:ready"
      )
        send();
    };
    window.addEventListener("message", onReady);
    send();
    return () => window.removeEventListener("message", onReady);
  }, [send]);
  return (
    <section className="preview-frame">
      <div className="frame-toolbar">
        <span className={`frame-dot ${original ? "" : "is-live"}`} />
        <span>
          {original
            ? t(($) => $.lab.preview.source)
            : t(($) => $.lab.preview.preview)}
        </span>
        <span className="ml-auto">
          {theme === "light"
            ? t(($) => $.lab.theme.light)
            : t(($) => $.lab.theme.dark)}
        </span>
      </div>
      <iframe
        ref={ref}
        src="?preview"
        title={
          original
            ? t(($) => $.lab.preview.originalFrame)
            : t(($) => $.lab.preview.editedFrame)
        }
        onLoad={send}
      />
    </section>
  );
}
const originalDraft = emptyDraft();

export function App() {
  const [locale, setLocale] = useState(loadLocale);
  return (
    <LabI18nProvider locale={locale}>
      <Workbench locale={locale} onLocaleChange={setLocale} />
    </LabI18nProvider>
  );
}

function Workbench({
  locale,
  onLocaleChange,
}: {
  locale: LabLocale;
  onLocaleChange: (locale: LabLocale) => void;
}) {
  const { t } = useTranslation("uiLab");
  const [session] = useState(loadSession);
  const [history, dispatch] = useReducer(editHistory, {
    past: [],
    present: session.draft,
    future: [],
  });
  const draft = history.present;
  const [theme, setTheme] = useState<Theme>("light");
  const [scene, setScene] = useState<Scene>("button");
  const [buttonScale, setButtonScale] = useState<ButtonScale>("default");
  const [compare, setCompare] = useState(false);
  const [original, setOriginal] = useState(false);
  const [numberPreview, setNumberPreview] = useState<{
    key: string;
    value: number;
  } | null>(null);
  const [colorPreview, setColorPreview] = useState<string | null>(null);
  const [color, setColor] = useState<string>("--brand");
  const [designs, setDesigns] = useState<SavedDesign[]>(session.designs);
  const [saveOpen, setSaveOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [name, setName] = useState("");
  const [notice, setNotice] = useState<{
    key: "saved" | "copied" | "copyFailed" | "exported" | "loaded";
    name?: string;
  } | null>(null);
  const [storageError, setStorageError] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const count = changeCount(draft);
  const currentScene = scenes.find((item) => item.id === scene)!;
  const currentColor = colorPreview ?? tokenValue(draft, theme, color);
  const previewDraft = colorPreview
    ? updateToken(draft, theme, color, colorPreview)
    : numberPreview
      ? updateToken(
          draft,
          "shared",
          numberPreview.key,
          `${numberPreview.value}px`,
        )
      : draft;
  const css = exportCss(draft);
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, encodeSession({ draft, designs }));
      localStorage.setItem(LOCALE_STORAGE_KEY, locale);
      setStorageError(false);
    } catch {
      setStorageError(true);
    }
  }, [draft, designs, locale]);
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 4000);
    return () => window.clearTimeout(timer);
  }, [notice]);
  const edit = (next: Draft) => {
    setColorPreview(null);
    setNumberPreview(null);
    dispatch({ type: "edit", draft: next });
    setOriginal(false);
  };
  const saveDesign = () => {
    if (!name.trim()) return;
    setDesigns(
      [
        {
          id: crypto.randomUUID(),
          name: name.trim(),
          draft,
          savedAt: new Date().toISOString(),
        },
        ...designs,
      ].slice(0, 20),
    );
    setName("");
    setSaveOpen(false);
    setNotice({ key: "saved" });
  };
  const copyCss = async () => {
    try {
      await navigator.clipboard.writeText(css);
      setNotice({ key: "copied" });
    } catch {
      setNotice({ key: "copyFailed" });
    }
  };
  const downloadCss = () => {
    const url = URL.createObjectURL(
      new Blob([css], { type: "text/css;charset=utf-8" }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "multica-ui-tokens.css";
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    setNotice({ key: "exported" });
  };
  return (
    <div className="lab-shell">
      <header className="lab-header">
        <Button
          className="mobile-nav-toggle"
          size="icon"
          variant="ghost"
          aria-label={t(($) => $.lab.nav.toggle)}
          onClick={() => setNavOpen(!navOpen)}
        >
          <PanelLeft />
        </Button>
        <a className="lab-brand" href="./" aria-label="Multica UI Lab">
          <span className="logo-mark">
            <MulticaIcon className="size-4" noSpin />
          </span>
          <strong>multica</strong>
          <span className="brand-divider" />
          <span>UI Lab</span>
        </a>
        <span className="internal-label">{t(($) => $.lab.nav.internal)}</span>
        <div className="header-actions">
          <div
            className="language-toggle"
            role="group"
            aria-label={t(($) => $.lab.language.label)}
          >
            {(["en", "zh"] as const).map((value) => (
              <Button
                key={value}
                size="sm"
                variant={locale === value ? "secondary" : "ghost"}
                aria-pressed={locale === value}
                lang={value === "zh" ? "zh-Hans" : "en"}
                onClick={() => onLocaleChange(value)}
              >
                {value === "en" ? "EN" : "中文"}
              </Button>
            ))}
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t(($) => $.lab.actions.undo)}
            title={t(($) => $.lab.actions.undo)}
            disabled={!history.past.length}
            onClick={() => dispatch({ type: "undo" })}
          >
            <Undo2 />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t(($) => $.lab.actions.redo)}
            title={t(($) => $.lab.actions.redo)}
            disabled={!history.future.length}
            onClick={() => dispatch({ type: "redo" })}
          >
            <Redo2 />
          </Button>
          <span className="header-separator" />
          <Button
            variant="outline"
            aria-label={t(($) => $.lab.actions.save)}
            onClick={() => setSaveOpen(true)}
          >
            <Save />
            <span>{t(($) => $.lab.actions.save)}</span>
          </Button>
          <Button
            aria-label={t(($) => $.lab.actions.export)}
            onClick={() => setExportOpen(true)}
          >
            <Code2 />
            <span>{t(($) => $.lab.actions.export)}</span>
          </Button>
        </div>
      </header>
      <aside className={`lab-nav ${navOpen ? "nav-open" : ""}`}>
        <div className="nav-heading">
          {t(($) => $.lab.nav.scenes)}
          <span>{String(scenes.length).padStart(2, "0")}</span>
        </div>
        <nav aria-label={t(($) => $.lab.nav.previewScenes)}>
          {scenes.map((item, index) => {
            const Icon = [LayoutGrid, MousePointer2, ListTodo, PanelLeft][
              index
            ]!;
            return (
              <button
                key={item.id}
                aria-current={scene === item.id ? "page" : undefined}
                onClick={() => {
                  setScene(item.id);
                  setNavOpen(false);
                }}
              >
                <Icon />
                <span>{t(($) => $.lab.scenes[item.label])}</span>
                {scene === item.id && <ChevronRight className="ml-auto" />}
              </button>
            );
          })}
        </nav>
        <div className="nav-heading saved-heading">
          {t(($) => $.lab.nav.saved)}
          <span>{String(designs.length).padStart(2, "0")}</span>
        </div>
        <div className="saved-designs">
          {designs.length ? (
            designs.map((design) => (
              <div className="saved-design" key={design.id}>
                <button
                  title={design.name}
                  onClick={() => {
                    edit(design.draft);
                    setNotice({ key: "loaded", name: design.name });
                  }}
                >
                  <span
                    className="design-swatch"
                    style={{
                      background: tokenValue(design.draft, "light", "--brand"),
                    }}
                  />
                  <span>{design.name}</span>
                </button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={t(($) => $.lab.actions.deleteDesign, {
                    name: design.name,
                  })}
                  onClick={() =>
                    setDesigns(designs.filter((item) => item.id !== design.id))
                  }
                >
                  <Trash2 />
                </Button>
              </div>
            ))
          ) : (
            <p className="nav-empty">{t(($) => $.lab.nav.empty)}</p>
          )}
        </div>
      </aside>
      <main className="lab-main">
        <div className="workspace-heading">
          <div>
            <div className="workspace-breadcrumb">
              {t(($) => $.lab.nav.workbench)}
              <ChevronRight /> {t(($) => $.lab.scenes[currentScene.label])}
            </div>
            <h1>{t(($) => $.lab.scenes[currentScene.label])}</h1>
          </div>
          <div
            className="theme-toggle"
            aria-label={t(($) => $.lab.theme.label)}
          >
            <Button
              variant={theme === "light" ? "secondary" : "ghost"}
              size="icon"
              aria-label={t(($) => $.lab.theme.lightLabel)}
              aria-pressed={theme === "light"}
              onClick={() => {
                setColorPreview(null);
                setTheme("light");
              }}
            >
              <Sun />
            </Button>
            <Button
              variant={theme === "dark" ? "secondary" : "ghost"}
              size="icon"
              aria-label={t(($) => $.lab.theme.darkLabel)}
              aria-pressed={theme === "dark"}
              onClick={() => {
                setColorPreview(null);
                setTheme("dark");
              }}
            >
              <Moon />
            </Button>
          </div>
        </div>
        <div className="canvas-toolbar">
          <span className="canvas-caption">
            <span className="status-dot" />
            {count
              ? t(($) => $.lab.preview.changeCount, { count })
              : t(($) => $.lab.preview.unchanged)}
          </span>
          <div className="ml-auto flex gap-1">
            <Button
              variant={original ? "secondary" : "ghost"}
              size="sm"
              aria-pressed={original}
              disabled={compare}
              onClick={() => setOriginal(!original)}
            >
              {original
                ? t(($) => $.lab.preview.back)
                : t(($) => $.lab.preview.original)}
            </Button>
            <Button
              variant={compare ? "secondary" : "ghost"}
              size="sm"
              aria-pressed={compare}
              onClick={() => {
                setCompare(!compare);
                setOriginal(false);
              }}
            >
              <Columns2 />
              {t(($) => $.lab.preview.compare)}
            </Button>
          </div>
        </div>
        <div className={`preview-canvas ${compare ? "compare" : ""}`}>
          {compare && (
            <PreviewFrame
              draft={originalDraft}
              theme={theme}
              scene={scene}
              buttonScale={buttonScale}
              original
            />
          )}
          <PreviewFrame
            draft={original ? originalDraft : previewDraft}
            theme={theme}
            scene={scene}
            buttonScale={buttonScale}
            original={original}
          />
        </div>
      </main>
      <Inspector
        scene={scene}
        theme={theme}
        draft={draft}
        previewDraft={previewDraft}
        buttonScale={buttonScale}
        color={color}
        currentColor={currentColor}
        onScaleChange={setButtonScale}
        onEdit={edit}
        onNumberPreview={(key, value) => {
          setColorPreview(null);
          setNumberPreview({ key, value });
        }}
        onNumberCancel={() => setNumberPreview(null)}
        onColorSelect={(key) => {
          setColorPreview(null);
          setNumberPreview(null);
          setColor(key);
        }}
        onColorPreview={setColorPreview}
        onColorCancel={() => setColorPreview(null)}
        onExport={() => setExportOpen(true)}
      />
      {storageError && (
        <div className="storage-warning" role="alert">
          {t(($) => $.lab.notice.storageFailed)}
        </div>
      )}
      {notice && (
        <div className="lab-notice" role="status">
          <Check className="size-4" />
          <span>
            {t(($) => $.lab.notice[notice.key], { name: notice.name })}
          </span>
          <button
            aria-label={t(($) => $.lab.notice.dismiss)}
            onClick={() => setNotice(null)}
          >
            <X className="size-3" />
          </button>
        </div>
      )}
      <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(($) => $.lab.save.title)}</DialogTitle>
            <DialogDescription>
              {t(($) => $.lab.save.description)}
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              saveDesign();
            }}
          >
            <label htmlFor="design-name" className="mb-2 block text-label">
              {t(($) => $.lab.save.name)}
            </label>
            <Input
              id="design-name"
              maxLength={60}
              value={name}
              placeholder={t(($) => $.lab.save.placeholder)}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
            <Button
              type="submit"
              className="mt-4 w-full"
              disabled={!name.trim()}
            >
              {t(($) => $.lab.actions.save)}
            </Button>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t(($) => $.lab.export.title)}</DialogTitle>
            <DialogDescription>
              {t(($) => $.lab.export.description)}
            </DialogDescription>
          </DialogHeader>
          <pre className="export-code" tabIndex={0}>
            <code>{css}</code>
          </pre>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={copyCss}>
              <Copy />
              {t(($) => $.lab.export.copy)}
            </Button>
            <Button onClick={downloadCss}>
              <ArrowDownToLine />
              {t(($) => $.lab.export.download)}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
