import { useState, type ReactNode } from "react";
import {
  ArrowDownToLine,
  Check,
  ChevronDown,
  Component,
  RotateCcw,
  X,
} from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@multica/ui/components/ui/tabs";
import {
  baseline,
  buttonScales,
  buttonTokens,
  changeCount,
  colorTokens,
  sizeTokens,
  sizeValue,
  tokenValue,
  updateToken,
  type ButtonScale,
  type Draft,
  type Scope,
  type Theme,
} from "./tokens";
import { type Scene } from "./protocol";
import { NumberField } from "./number-field";
import { ColorEditor } from "./color-editor";
import { colorToHex } from "./color";

function PropertySection({
  title,
  scope,
  children,
  actions,
}: {
  title: string;
  scope?: string;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section className="property-section">
      <header>
        <h3>{title}</h3>
        {scope && <span className="property-scope">{scope}</span>}
        {actions}
      </header>
      {children}
    </section>
  );
}

export function Inspector({
  scene,
  theme,
  draft,
  previewDraft,
  buttonScale,
  color,
  currentColor,
  onScaleChange,
  onEdit,
  onNumberPreview,
  onNumberCancel,
  onColorSelect,
  onColorPreview,
  onColorCancel,
  onExport,
}: {
  scene: Scene;
  theme: Theme;
  draft: Draft;
  previewDraft: Draft;
  buttonScale: ButtonScale;
  color: string;
  currentColor: string;
  onScaleChange: (scale: ButtonScale) => void;
  onEdit: (draft: Draft) => void;
  onNumberPreview: (key: string, value: number) => void;
  onNumberCancel: () => void;
  onColorSelect: (token: string) => void;
  onColorPreview: (value: string) => void;
  onColorCancel: () => void;
  onExport: () => void;
}) {
  const [tab, setTab] = useState("design");
  const [openColor, setOpenColor] = useState<string | null>(null);
  const count = changeCount(draft);
  const reset = (scope: Scope, key: string) =>
    onEdit(updateToken(draft, scope, key, baseline[scope][key]!));
  const symbols: Record<string, string> = {
    radius: "⌜",
    type: "T",
    density: "H",
  };
  const number = (token: (typeof sizeTokens)[number]) => (
    <NumberField
      key={token.key}
      label={token.label}
      min={token.min}
      max={token.max}
      step={token.step}
      unit={token.unit}
      token={token.key}
      symbol={
        token.key.includes("height")
          ? "H"
          : token.key.includes("padding")
            ? "↔"
            : token.key.includes("gap")
              ? "⋮"
              : (symbols[token.group] ?? "#")
      }
      value={sizeValue(tokenValue(previewDraft, "shared", token.key))}
      modified={!!draft.shared[token.key]}
      onPreview={(value) => onNumberPreview(token.key, value)}
      onCommit={(value) =>
        onEdit(updateToken(draft, "shared", token.key, `${value}px`))
      }
      onCancel={onNumberCancel}
      onReset={() => reset("shared", token.key)}
    />
  );
  const prominent =
    scene === "button"
      ? ["--primary", "--primary-foreground", "--brand", "--destructive"]
      : ["--page-canvas", "--surface", "--foreground", "--brand"];
  const colorRow = ([key, label]: (typeof colorTokens)[number]) => {
    const value = color === key ? currentColor : tokenValue(draft, theme, key);
    return (
      <Popover
        key={key}
        open={openColor === key}
        onOpenChange={(open) => {
          setOpenColor(open ? key : null);
          if (open) onColorSelect(key);
          else onColorCancel();
        }}
      >
        <PopoverTrigger
          render={
            <button
              type="button"
              className="property-color-row"
              aria-label={`编辑${label}`}
            />
          }
        >
          <span className="property-color-label" title={key}>
            {label}
          </span>
          <span className="property-color-value">
            <span
              className="property-color-chip"
              style={{ background: value }}
            />
            <code>{colorToHex(value).slice(1)}</code>
            {draft[theme][key] && (
              <span
                className="property-modified"
                aria-hidden="true"
                title="已修改"
              />
            )}
          </span>
        </PopoverTrigger>
        <PopoverContent
          side="left"
          align="start"
          sideOffset={12}
          className="property-color-popup"
          aria-label={`${label}颜色编辑器`}
        >
          <div className="property-popup-heading">
            <strong>{label}</strong>
            <span>{theme === "light" ? "浅色" : "深色"}</span>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="关闭颜色编辑器"
              onClick={() => {
                setOpenColor(null);
                onColorCancel();
              }}
            >
              <X />
            </Button>
          </div>
          <ColorEditor
            key={`${theme}:${key}`}
            showRoleSelector={false}
            token={key}
            value={value}
            original={baseline[theme][key]!}
            modified={!!draft[theme][key]}
            values={Object.fromEntries(
              colorTokens.map(([token]) => [
                token,
                tokenValue(draft, theme, token),
              ]),
            )}
            onTokenChange={onColorSelect}
            onPreview={onColorPreview}
            onCommit={(next) => onEdit(updateToken(draft, theme, key, next))}
          />
        </PopoverContent>
      </Popover>
    );
  };
  const changes = (
    Object.entries(draft) as [Scope, Record<string, string>][]
  ).flatMap(([scope, values]) =>
    Object.entries(values).map(([key, value]) => ({ scope, key, value })),
  );
  return (
    <aside className="lab-inspector property-inspector" aria-label="设计参数">
      <Tabs
        value={tab}
        onValueChange={(value) => {
          setTab(value);
          onNumberCancel();
          onColorCancel();
        }}
        className="property-tabs"
      >
        <div className="property-tabs-header">
          <TabsList variant="line" aria-label="属性面板">
            <TabsTrigger value="design">设计</TabsTrigger>
            <TabsTrigger value="changes">
              修改{count > 0 && <span className="property-count">{count}</span>}
            </TabsTrigger>
          </TabsList>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="重置全部修改"
            title="重置全部修改（可撤销）"
            disabled={!count}
            onClick={() => onEdit({ light: {}, dark: {}, shared: {} })}
          >
            <RotateCcw />
          </Button>
        </div>
        <TabsContent value="design" className="property-tab-panel">
          <div className="property-object">
            <div>
              <Component className="size-4" />
              <strong>{scene === "button" ? "Button" : "设计系统"}</strong>
            </div>
          </div>
          {scene === "button" && (
            <PropertySection
              title="尺寸"
              scope="组件"
              actions={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label="重置按钮参数"
                  title="重置全部按钮尺寸"
                  disabled={
                    !buttonTokens.some((token) => draft.shared[token.key])
                  }
                  onClick={() =>
                    onEdit({
                      ...draft,
                      shared: Object.fromEntries(
                        Object.entries(draft.shared).filter(
                          ([key]) => !key.startsWith("--button-"),
                        ),
                      ),
                    })
                  }
                >
                  <RotateCcw />
                </Button>
              }
            >
              <div
                className="property-segments"
                role="group"
                aria-label="调整尺寸"
              >
                {buttonScales.map((scale) => (
                  <button
                    key={scale}
                    type="button"
                    aria-label={`尺寸 ${scale}`}
                    aria-pressed={buttonScale === scale}
                    onClick={() => onScaleChange(scale)}
                  >
                    {scale === "default"
                      ? "M"
                      : scale === "sm"
                        ? "S"
                        : scale.toUpperCase()}
                  </button>
                ))}
              </div>
              <div className="property-grid">
                {buttonTokens
                  .filter((token) => token.scale === buttonScale)
                  .map(number)}
              </div>
            </PropertySection>
          )}
          <PropertySection title="外观" scope="全局">
            <div className="property-grid">
              {sizeTokens
                .filter((token) => token.group === "radius")
                .map(number)}
              <div className="property-radius-preview" aria-hidden="true">
                <span
                  style={{
                    borderRadius: sizeValue(
                      tokenValue(previewDraft, "shared", "--radius"),
                    ),
                  }}
                />
              </div>
            </div>
          </PropertySection>
          <PropertySection
            title="颜色"
            scope={`全局 · ${theme === "light" ? "浅色" : "深色"}`}
          >
            <div className="property-colors">
              {colorTokens
                .filter(([key]) => prominent.includes(key))
                .map(colorRow)}
            </div>
            <details className="property-more-colors">
              <summary>
                更多颜色
                <ChevronDown className="size-3" />
              </summary>
              <div className="property-colors">
                {colorTokens
                  .filter(([key]) => !prominent.includes(key))
                  .map(colorRow)}
              </div>
            </details>
          </PropertySection>
          <PropertySection title="文字" scope="全局">
            <div className="property-grid">
              {sizeTokens.filter((token) => token.group === "type").map(number)}
            </div>
          </PropertySection>
          {scene !== "button" && (
            <PropertySection title="列表密度" scope="全局">
              <div className="property-grid">
                {sizeTokens
                  .filter((token) => token.group === "density")
                  .map(number)}
              </div>
            </PropertySection>
          )}
        </TabsContent>
        <TabsContent value="changes" className="property-tab-panel">
          <div className="property-changes-intro">
            <strong>{count ? `${count} 项修改` : "与原版一致"}</strong>
          </div>
          {changes.map(({ scope, key, value }) => (
            <div className="property-change" key={`${scope}:${key}`}>
              <header>
                <strong>
                  {sizeTokens.find((token) => token.key === key)?.label ??
                    colorTokens.find(([token]) => token === key)?.[1] ??
                    key}
                </strong>
                <span>
                  {scope === "shared"
                    ? "共享"
                    : scope === "light"
                      ? "浅色"
                      : "深色"}
                </span>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`恢复 ${key} ${scope}`}
                  onClick={() => reset(scope, key)}
                >
                  <RotateCcw />
                </Button>
              </header>
              <code>{key}</code>
              <div className="property-change-values">
                <del>{baseline[scope][key]}</del>
                <span>{value}</span>
              </div>
            </div>
          ))}
          {!count && (
            <div className="property-empty">
              <Check className="size-6" />
              <span>所有参数均使用当前代码的值</span>
            </div>
          )}
        </TabsContent>
      </Tabs>
      <div className="property-export">
        <Button variant="outline" onClick={onExport}>
          <ArrowDownToLine />
          导出 CSS<span>{count || ""}</span>
        </Button>
      </div>
    </aside>
  );
}
