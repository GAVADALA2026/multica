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
  const ref = useRef<HTMLIFrameElement>(null);
  const send = useCallback(() => {
    ref.current?.contentWindow?.postMessage(
      {
        type: "multica-ui-lab:preview",
        draft,
        theme,
        scene,
        buttonScale,
      } satisfies PreviewSettings,
      location.origin,
    );
  }, [draft, theme, scene, buttonScale]);
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
        <span>{original ? "当前代码" : "预览"}</span>
        <span className="ml-auto">{theme === "light" ? "浅色" : "深色"}</span>
      </div>
      <iframe
        ref={ref}
        src="?preview"
        title={original ? "原版界面" : "调整后的界面"}
        onLoad={send}
      />
    </section>
  );
}
const originalDraft = emptyDraft();

export function App() {
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
  const [notice, setNotice] = useState("");
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
      setStorageError(false);
    } catch {
      setStorageError(true);
    }
  }, [draft, designs]);
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(""), 4000);
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
    setNotice("方案已保存到此浏览器");
  };
  const copyCss = async () => {
    try {
      await navigator.clipboard.writeText(css);
      setNotice("CSS 已复制");
    } catch {
      setNotice("无法访问剪贴板，请选择代码手动复制，或下载 CSS");
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
    setNotice("CSS 已导出");
  };
  return (
    <div className="lab-shell">
      <header className="lab-header">
        <Button
          className="mobile-nav-toggle"
          size="icon"
          variant="ghost"
          aria-label="切换场景导航"
          onClick={() => setNavOpen(!navOpen)}
        >
          <PanelLeft />
        </Button>
        <a className="lab-brand" href="./">
          <span className="logo-mark">
            <MulticaIcon className="size-4" noSpin />
          </span>
          <strong>multica</strong>
          <span className="brand-divider" />
          <span>UI Lab</span>
        </a>
        <span className="internal-label">内部工作台</span>
        <div className="header-actions">
          <Button
            variant="ghost"
            size="icon"
            aria-label="撤销"
            title="撤销"
            disabled={!history.past.length}
            onClick={() => dispatch({ type: "undo" })}
          >
            <Undo2 />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="重做"
            title="重做"
            disabled={!history.future.length}
            onClick={() => dispatch({ type: "redo" })}
          >
            <Redo2 />
          </Button>
          <span className="header-separator" />
          <Button variant="outline" onClick={() => setSaveOpen(true)}>
            <Save />
            <span>保存方案</span>
          </Button>
          <Button onClick={() => setExportOpen(true)}>
            <Code2 />
            <span>导出修改</span>
          </Button>
        </div>
      </header>
      <aside className={`lab-nav ${navOpen ? "nav-open" : ""}`}>
        <div className="nav-heading">
          组件与场景<span>{String(scenes.length).padStart(2, "0")}</span>
        </div>
        <nav aria-label="预览场景">
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
                <span>{item.label}</span>
                {scene === item.id && <ChevronRight className="ml-auto" />}
              </button>
            );
          })}
        </nav>
        <div className="nav-heading saved-heading">
          已存方案<span>{String(designs.length).padStart(2, "0")}</span>
        </div>
        <div className="saved-designs">
          {designs.length ? (
            designs.map((design) => (
              <div className="saved-design" key={design.id}>
                <button
                  title={design.name}
                  onClick={() => {
                    edit(design.draft);
                    setNotice(`已载入：${design.name}`);
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
                  aria-label={`删除方案 ${design.name}`}
                  onClick={() =>
                    setDesigns(designs.filter((item) => item.id !== design.id))
                  }
                >
                  <Trash2 />
                </Button>
              </div>
            ))
          ) : (
            <p className="nav-empty">暂无方案</p>
          )}
        </div>
      </aside>
      <main className="lab-main">
        <div className="workspace-heading">
          <div>
            <div className="workspace-breadcrumb">
              工作台 <ChevronRight /> {currentScene.label}
            </div>
            <h1>{currentScene.label}</h1>
          </div>
          <div className="theme-toggle" aria-label="预览主题">
            <Button
              variant={theme === "light" ? "secondary" : "ghost"}
              size="icon"
              aria-label="浅色主题"
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
              aria-label="深色主题"
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
            {count ? `${count} 项修改` : "与当前代码一致"}
          </span>
          <div className="ml-auto flex gap-1">
            <Button
              variant={original ? "secondary" : "ghost"}
              size="sm"
              aria-pressed={original}
              disabled={compare}
              onClick={() => setOriginal(!original)}
            >
              {original ? "返回修改版" : "查看原版"}
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
              并排对比
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
          浏览器无法保存草稿，请导出 CSS 以保留修改。
        </div>
      )}
      {notice && (
        <div className="lab-notice" role="status">
          <Check className="size-4" />
          <span>{notice}</span>
          <button aria-label="关闭提示" onClick={() => setNotice("")}>
            <X className="size-3" />
          </button>
        </div>
      )}
      <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存设计方案</DialogTitle>
            <DialogDescription>
              保存在当前浏览器，最多 20 份。
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              saveDesign();
            }}
          >
            <label htmlFor="design-name" className="mb-2 block text-label">
              方案名称
            </label>
            <Input
              id="design-name"
              maxLength={60}
              value={name}
              placeholder="例如：更紧凑的任务视图"
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
            <Button
              type="submit"
              className="mt-4 w-full"
              disabled={!name.trim()}
            >
              保存方案
            </Button>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>导出设计修改</DialogTitle>
            <DialogDescription>
              合并到 packages/ui/styles/tokens.css
              的对应代码块。导出不会写回源码。
            </DialogDescription>
          </DialogHeader>
          <pre className="export-code" tabIndex={0}>
            <code>{css}</code>
          </pre>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={copyCss}>
              <Copy />
              复制 CSS
            </Button>
            <Button onClick={downloadCss}>
              <ArrowDownToLine />
              下载 CSS
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
