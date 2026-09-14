import { useEffect, useRef, useState, type ComponentProps } from "react";
import {
  ArrowRight,
  Check,
  ChevronDown,
  Loader2,
  Plus,
  Send,
} from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import {
  buttonScales,
  sizeValue,
  tokenValue,
  type ButtonScale,
  type Draft,
} from "./tokens";

type Variant = NonNullable<ComponentProps<typeof Button>["variant"]>;
type Size = NonNullable<ComponentProps<typeof Button>["size"]>;
const variants = {
  default: { label: "主要操作" },
  outline: { label: "次要操作" },
  secondary: { label: "柔和填充" },
  ghost: { label: "轻量操作" },
  brand: { label: "品牌强调" },
  brandSubtle: {
    label: "品牌浅色",
  },
  destructive: {
    label: "危险操作",
  },
  link: { label: "链接样式" },
} satisfies Record<Variant, { label: string }>;
const entries = Object.entries(variants) as [
  Variant,
  (typeof variants)[Variant],
][];
const iconSizes = {
  xs: "icon-xs",
  sm: "icon-sm",
  default: "icon",
  lg: "icon-lg",
} as const satisfies Record<ButtonScale, Size>;

export function ButtonScene({
  scale,
  draft,
}: {
  scale: ButtonScale;
  draft: Draft;
}) {
  const [variant, setVariant] = useState<Variant>("default");
  const [label, setLabel] = useState("创建任务");
  const [state, setState] = useState("normal");
  const [icon, setIcon] = useState("start");
  const [clicks, setClicks] = useState(0);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const loading = state === "loading";
  const buttonLabel = label.trim() || "创建任务";
  const size = (key: string) =>
    `${sizeValue(tokenValue(draft, "shared", key))}px`;
  return (
    <div className="button-scene">
      <header className="button-intro">
        <h1>
          Button <span>按钮</span>
        </h1>
        <div className="button-meta">
          <span>8 种外观</span>
          <span>8 种尺寸</span>
        </div>
      </header>
      <nav className="button-section-nav" aria-label="按钮页面目录">
        <a href="#button-playground">试用</a>
        <a href="#button-variants">外观与状态</a>
        <a href="#button-sizes">尺寸</a>
        <a href="#button-context">使用场景</a>
      </nav>

      <section
        className="button-section"
        id="button-playground"
        aria-labelledby="playground-title"
      >
        <div className="button-section-heading">
          <div>
            <h2 id="playground-title">试用按钮</h2>
          </div>
          <code>{scale}</code>
        </div>
        <div className="button-playground-controls">
          <label>
            按钮文字
            <Input
              aria-label="按钮文字"
              value={label}
              maxLength={100}
              onChange={(event) => setLabel(event.target.value)}
            />
          </label>
          <label>
            外观
            <select
              aria-label="外观"
              value={variant}
              onChange={(event) => setVariant(event.target.value as Variant)}
            >
              {entries.map(([key, value]) => (
                <option key={key} value={key}>
                  {value.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            内容
            <select
              aria-label="内容"
              value={icon}
              onChange={(event) => setIcon(event.target.value)}
            >
              <option value="start">前置图标</option>
              <option value="end">后置图标</option>
              <option value="none">仅文字</option>
              <option value="only">仅图标</option>
            </select>
          </label>
          <label>
            状态
            <select
              aria-label="状态"
              value={state}
              onChange={(event) => setState(event.target.value)}
            >
              <option value="normal">默认</option>
              <option value="disabled">禁用</option>
              <option value="loading">加载中</option>
              <option value="invalid">无效输入</option>
            </select>
          </label>
        </div>
        <div className="button-stage">
          <Button
            data-testid="button-playground"
            variant={variant}
            size={icon === "only" ? iconSizes[scale] : scale}
            disabled={state === "disabled" || loading}
            aria-busy={loading || undefined}
            aria-invalid={state === "invalid" || undefined}
            aria-label={icon === "only" ? buttonLabel : undefined}
            onClick={() => setClicks((value) => value + 1)}
          >
            {loading ? (
              <Loader2 className="animate-spin" />
            ) : icon === "start" || icon === "only" ? (
              <Plus data-icon={icon === "start" ? "inline-start" : undefined} />
            ) : null}
            {icon !== "only" && buttonLabel}
            {!loading && icon === "end" && (
              <ArrowRight data-icon="inline-end" />
            )}
          </Button>
          <p role="status">{clicks ? `已触发 ${clicks} 次示例操作` : ""}</p>
        </div>
        <div className="button-stage-footer">
          <span>高度 {size(`--button-height-${scale}`)}</span>
          <span>内边距 {size(`--button-padding-${scale}`)}</span>
          <span>图文间距 {size(`--button-gap-${scale}`)}</span>
        </div>
      </section>

      <section
        className="button-section"
        id="button-variants"
        aria-labelledby="variants-title"
      >
        <div className="button-section-heading">
          <div>
            <h2 id="variants-title">外观与状态</h2>
          </div>
          <code>{scale}</code>
        </div>
        <div className="button-table-scroll">
          <table className="button-matrix">
            <thead>
              <tr>
                <th scope="col">外观</th>
                <th scope="col">默认 / 可交互</th>
                <th scope="col">禁用</th>
                <th scope="col">加载中</th>
              </tr>
            </thead>
            <tbody>
              {entries.map(([key, value]) => (
                <tr key={key} data-variant={key}>
                  <th scope="row">
                    <code>{key}</code>
                  </th>
                  <td>
                    <Button
                      variant={key}
                      size={scale}
                      onClick={() => setClicks((n) => n + 1)}
                    >
                      {value.label}
                    </Button>
                  </td>
                  <td>
                    <Button variant={key} size={scale} disabled>
                      {value.label}
                    </Button>
                  </td>
                  <td>
                    <Button
                      variant={key}
                      size={scale}
                      disabled
                      aria-busy="true"
                    >
                      <Loader2 className="animate-spin" />
                      处理中
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section
        className="button-section"
        id="button-sizes"
        aria-labelledby="sizes-title"
      >
        <div className="button-section-heading">
          <div>
            <h2 id="sizes-title">尺寸与图标</h2>
          </div>
        </div>
        <div className="button-table-scroll">
          <table className="button-matrix button-size-matrix">
            <thead>
              <tr>
                <th scope="col">尺寸</th>
                <th scope="col">文字</th>
                <th scope="col">前置图标</th>
                <th scope="col">后置图标</th>
                <th scope="col">图标</th>
              </tr>
            </thead>
            <tbody>
              {buttonScales.map((item) => (
                <tr key={item} data-size={item} data-selected={item === scale}>
                  <th scope="row">
                    <code>{item}</code>
                    <span>{size(`--button-height-${item}`)}</span>
                  </th>
                  <td>
                    <Button size={item} variant={variant}>
                      创建任务
                    </Button>
                  </td>
                  <td>
                    <Button size={item} variant={variant}>
                      <Plus data-icon="inline-start" />
                      创建任务
                    </Button>
                  </td>
                  <td>
                    <Button size={item} variant={variant}>
                      继续
                      <ArrowRight data-icon="inline-end" />
                    </Button>
                  </td>
                  <td>
                    <Button
                      size={iconSizes[item]}
                      variant={variant}
                      aria-label={`添加任务 ${iconSizes[item]}`}
                    >
                      <Plus />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section
        className="button-section"
        id="button-context"
        aria-labelledby="context-title"
      >
        <div className="button-section-heading">
          <div>
            <h2 id="context-title">使用场景</h2>
          </div>
        </div>
        <div className="button-context-grid">
          <div className="button-context-card">
            <h3>提交与反馈</h3>
            <div className="button-example-actions">
              <Button
                size={scale}
                disabled={sending}
                aria-busy={sending || undefined}
                onClick={() => {
                  setSending(true);
                  setSent(false);
                  timer.current = setTimeout(() => {
                    setSending(false);
                    setSent(true);
                  }, 1200);
                }}
              >
                {sending ? (
                  <Loader2 className="animate-spin" />
                ) : sent ? (
                  <Check />
                ) : (
                  <Send />
                )}
                {sending ? "提交中..." : sent ? "已提交" : "提交任务"}
              </Button>
              <span
                className="text-caption text-muted-foreground"
                role="status"
              >
                {sent ? "示例提交完成" : ""}
              </span>
            </div>
          </div>
          <div className="button-context-card">
            <h3>弹层触发器</h3>
            <div className="button-example-actions">
              <Popover>
                <PopoverTrigger
                  render={<Button variant="outline" size={scale} />}
                >
                  更多操作
                  <ChevronDown data-icon="inline-end" />
                </PopoverTrigger>
                <PopoverContent>
                  <p className="text-body">弹层内容</p>
                </PopoverContent>
              </Popover>
              <Dialog>
                <DialogTrigger
                  render={<Button variant="secondary" size={scale} />}
                >
                  打开对话框
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>保存修改</DialogTitle>
                    <DialogDescription>保存当前修改。</DialogDescription>
                  </DialogHeader>
                  <Button size={scale} onClick={() => setClicks((n) => n + 1)}>
                    保存示例
                  </Button>
                </DialogContent>
              </Dialog>
            </div>
          </div>
          <div className="button-context-card">
            <h3>文字长度</h3>
            <div className="button-example-actions">
              <Button variant="outline" size={scale}>
                Save changes
              </Button>
              <Button size={scale}>保存并继续创建下一项任务</Button>
            </div>
          </div>
          <div className="button-context-card">
            <h3>紧凑空间</h3>
            <div className="button-example-actions">
              <Button
                variant="ghost"
                size="icon"
                className="size-6"
                aria-label="紧凑添加"
              >
                <Plus className="size-3" />
              </Button>
              <Button variant="outline" size={scale} className="h-7 px-2">
                快捷操作
              </Button>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
