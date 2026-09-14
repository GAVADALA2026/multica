import { useEffect, useState, type ReactNode } from "react";
import { ArrowUpRight, Check, Inbox, Plus } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { Input } from "@multica/ui/components/ui/input";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Switch } from "@multica/ui/components/ui/switch";
import { Avatar, AvatarFallback } from "@multica/ui/components/ui/avatar";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import { StatusIcon } from "@multica/views/issues/visuals";
import { ButtonScene } from "./button-scene";
import { ProductPreview } from "./product-preview";
import { emptyDraft, previewCss } from "./tokens";
import { isPreviewSettings, type PreviewSettings } from "./protocol";

function Specimen({
  title,
  caption,
  children,
  wide = false,
}: {
  title: string;
  caption: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <section className={`specimen ${wide ? "specimen-wide" : ""}`}>
      <header>
        <h2>{title}</h2>
        <span>{caption}</span>
      </header>
      <div className="specimen-body">{children}</div>
    </section>
  );
}
function Person({ name = "JZ" }: { name?: string }) {
  return (
    <Avatar size="sm">
      <AvatarFallback>{name}</AvatarFallback>
    </Avatar>
  );
}
function SurfaceSwatches() {
  const surfaces = [
    ["--app-shell", "框架"],
    ["--page-canvas", "页面"],
    ["--surface", "表面"],
    ["--surface-raised", "浮层"],
    ["--surface-hover", "悬停"],
    ["--surface-selected", "选中"],
  ];
  return (
    <div className="surface-swatches">
      {surfaces.map(([key, name]) => (
        <div key={key}>
          <div style={{ background: `var(${key})` }}>Aa</div>
          <span>{name}</span>
        </div>
      ))}
    </div>
  );
}
function ExampleDialog() {
  return (
    <Dialog>
      <DialogTrigger render={<Button variant="outline" />}>
        打开弹窗 <ArrowUpRight />
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建任务</DialogTitle>
          <DialogDescription>填写任务标题。</DialogDescription>
        </DialogHeader>
        <Input aria-label="弹窗任务标题" placeholder="输入任务标题..." />
      </DialogContent>
    </Dialog>
  );
}
function ComponentsScene() {
  return (
    <div className="component-scene">
      <div className="scene-intro">
        <h1>基础组件</h1>
      </div>
      <div className="specimen-grid">
        <Specimen title="表面与层次" caption="SURFACES" wide>
          <SurfaceSwatches />
        </Specimen>
        <Specimen title="按钮" caption="BUTTON">
          <div className="flex flex-wrap gap-3">
            <Button>创建任务</Button>
            <Button variant="brand">
              <Plus />
              新建
            </Button>
            <Button variant="secondary">次要操作</Button>
            <Button variant="outline">边框按钮</Button>
            <Button variant="ghost">轻量操作</Button>
            <Button variant="destructive">删除</Button>
            <Button disabled>不可用</Button>
          </div>
        </Specimen>
        <Specimen title="输入与选择" caption="FORM">
          <div className="grid gap-4">
            <Input
              aria-label="组件任务标题"
              placeholder="为任务起一个名字..."
            />
            <div className="flex items-center justify-between gap-4">
              <label className="flex items-center gap-2 text-body">
                <Checkbox defaultChecked />
                通知负责人
              </label>
              <label className="flex items-center gap-2 text-body">
                自动化 <Switch defaultChecked />
              </label>
            </div>
            <Input aria-label="禁用输入框" disabled placeholder="只读状态" />
          </div>
        </Specimen>
        <Specimen title="状态与标签" caption="STATUS">
          <div className="flex flex-wrap gap-x-5 gap-y-4">
            {[
              "backlog",
              "todo",
              "in_progress",
              "in_review",
              "done",
              "blocked",
            ].map((status) => (
              <span
                key={status}
                className="flex items-center gap-2 text-caption"
              >
                <StatusIcon status={status} />
                {status}
              </span>
            ))}
          </div>
          <div className="mt-5 flex flex-wrap gap-2">
            <Badge>Design</Badge>
            <Badge variant="secondary">Frontend</Badge>
            <Badge variant="outline">v0.4.0</Badge>
            <Badge variant="destructive">需关注</Badge>
          </div>
        </Specimen>
        <Specimen title="文字层级" caption="TYPOGRAPHY">
          <div className="grid gap-2">
            <h3 className="text-title-lg font-semibold">小细节，构成好体验</h3>
            <p className="text-body">
              Build with intention. 让智能体和团队一起工作。
            </p>
            <p className="text-label">任务属性 · 负责人 · 项目</p>
            <p className="text-caption text-muted-foreground">
              次要信息也应该清楚可读 · 2 分钟前
            </p>
          </div>
        </Specimen>
        <Specimen title="浮层与圆角" caption="ELEVATION">
          <div className="rounded-xl border border-surface-border bg-surface p-4 shadow-[var(--surface-shadow)]">
            <div className="mb-4 flex items-center gap-3">
              <Person />
              <div>
                <p className="text-body font-medium">产品体验优化</p>
                <p className="text-caption text-muted-foreground">
                  内外层圆角的关系
                </p>
              </div>
            </div>
            <ExampleDialog />
          </div>
        </Specimen>
        <Specimen title="加载与空状态" caption="FEEDBACK">
          <div className="flex items-center gap-3">
            <Skeleton className="size-8 rounded-full" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-3 w-3/5" />
              <Skeleton className="h-3 w-2/5" />
            </div>
          </div>
          <div className="mt-5 flex items-center gap-3 text-muted-foreground">
            <Inbox className="size-5" />
            <span className="text-body">所有通知都已处理</span>
            <Check className="ml-auto size-4 text-success" />
          </div>
        </Specimen>
      </div>
    </div>
  );
}
export function Preview() {
  const [settings, setSettings] = useState<PreviewSettings>({
    type: "multica-ui-lab:preview",
    draft: emptyDraft(),
    theme: "light",
    scene: "components",
    buttonScale: "default",
  });
  useEffect(() => {
    const receive = (event: MessageEvent<unknown>) => {
      if (
        event.origin === location.origin &&
        event.source === window.parent &&
        isPreviewSettings(event.data)
      )
        setSettings(event.data);
    };
    window.addEventListener("message", receive);
    window.parent.postMessage(
      { type: "multica-ui-lab:ready" },
      location.origin,
    );
    return () => window.removeEventListener("message", receive);
  }, []);
  useEffect(() => {
    document.documentElement.classList.toggle(
      "dark",
      settings.theme === "dark",
    );
  }, [settings.theme]);
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [settings.scene]);
  return (
    <div className="preview-root">
      <style>{previewCss(settings.draft)}</style>
      {settings.scene === "components" ? (
        <ComponentsScene />
      ) : settings.scene === "button" ? (
        <ButtonScene scale={settings.buttonScale} draft={settings.draft} />
      ) : settings.scene === "list" ? (
        <ProductPreview key="list" scene="list" />
      ) : (
        <ProductPreview key="detail" scene="detail" />
      )}
    </div>
  );
}
