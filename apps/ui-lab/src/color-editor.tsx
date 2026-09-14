import { useEffect, useId, useState } from "react";
import { HexColorPicker } from "react-colorful";
import { Check, ChevronDown, Copy, RotateCcw, Search } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { colorToHex, hexToOklch, isSrgb } from "./color";
import { colorTokens, parseColor } from "./tokens";

const presets = [
  "#FFFFFF",
  "#18181B",
  "#71717A",
  "#2563EB",
  "#7C3AED",
  "#DB2777",
  "#DC2626",
  "#D97706",
  "#16A34A",
];

export function ColorEditor({
  showRoleSelector = true,
  token,
  value,
  original,
  modified,
  values,
  onTokenChange,
  onPreview,
  onCommit,
}: {
  showRoleSelector?: boolean;
  token: string;
  value: string;
  original: string;
  modified: boolean;
  values: Record<string, string>;
  onTokenChange: (token: string) => void;
  onPreview: (color: string) => void;
  onCommit: (color: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [hexInput, setHexInput] = useState(colorToHex(value));
  const [error, setError] = useState("");
  const [copyState, setCopyState] = useState("");
  const errorId = useId();
  const hex = colorToHex(value);
  const channels = parseColor(value);
  const label = colorTokens.find(([key]) => key === token)![1];
  useEffect(() => {
    setHexInput(hex);
    setError("");
  }, [hex]);
  useEffect(() => {
    if (!copyState) return;
    const timeout = setTimeout(() => setCopyState(""), 2000);
    return () => clearTimeout(timeout);
  }, [copyState]);
  const commitHex = () => {
    const next = hexToOklch(hexInput);
    if (!next) {
      setError("请输入 3 位或 6 位 HEX 色值。");
      return;
    }
    setError("");
    setHexInput(colorToHex(next));
    // Focusing/blurring an approximate HEX display must not rewrite a wide-gamut token.
    if (colorToHex(next) !== hex) onCommit(next);
  };
  return (
    <div className="color-editor">
      {showRoleSelector && (
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (!next) setQuery("");
          }}
        >
          <PopoverTrigger
            render={
              <Button
                variant="outline"
                className="color-role-trigger"
                aria-label={`颜色角色：${label}`}
              />
            }
          >
            <span className="color-role-swatch" style={{ background: value }} />
            <span className="color-role-label">
              <strong>{label}</strong>
              <code>{token}</code>
            </span>
            <ChevronDown className="size-3.5 shrink-0" />
          </PopoverTrigger>
          <PopoverContent className="color-role-popover" align="end">
            <div className="color-role-search">
              <Search className="size-3.5" />
              <Input
                aria-label="搜索颜色角色"
                placeholder="搜索颜色或变量..."
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <div className="color-role-list" aria-label="颜色角色">
              {colorTokens
                .filter(([key, name]) =>
                  `${key} ${name}`.toLowerCase().includes(query.toLowerCase()),
                )
                .map(([key, name]) => (
                  <Button
                    key={key}
                    variant="ghost"
                    className="color-role-option"
                    aria-pressed={token === key}
                    onClick={() => {
                      onTokenChange(key);
                      setOpen(false);
                      setQuery("");
                    }}
                  >
                    <span
                      className="color-role-swatch"
                      style={{ background: values[key] }}
                    />
                    <span className="color-role-label">
                      <strong>{name}</strong>
                      <code>{key}</code>
                    </span>
                    {key === token && <Check className="size-3.5" />}
                  </Button>
                ))}
              {!colorTokens.some(([key, name]) =>
                `${key} ${name}`.toLowerCase().includes(query.toLowerCase()),
              ) && <p className="color-role-empty">没有找到匹配的颜色。</p>}
            </div>
          </PopoverContent>
        </Popover>
      )}
      <div className="color-picker-panel">
        <HexColorPicker
          color={hex}
          onChange={(next) => onPreview(hexToOklch(next)!)}
          onChangeEnd={(next) => onCommit(hexToOklch(next)!)}
          aria-label="颜色色板"
        />
      </div>
      <div className="color-hex-row">
        <label className="color-hex-field">
          <span>HEX</span>
          <Input
            aria-label="HEX 色值"
            value={hexInput}
            maxLength={7}
            spellCheck={false}
            aria-invalid={!!error}
            aria-describedby={error ? errorId : undefined}
            onChange={(event) => {
              setHexInput(event.target.value);
              setError("");
            }}
            onBlur={commitHex}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                commitHex();
              }
              if (event.key === "Escape") {
                setHexInput(hex);
                setError("");
              }
            }}
          />
        </label>
        <Button
          variant="outline"
          size="icon"
          aria-label="复制 HEX 色值"
          title="复制 HEX 色值"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(hex);
              setCopyState("已复制色值");
            } catch {
              setCopyState("无法复制，请选择色值后手动复制。");
            }
          }}
        >
          {copyState === "已复制色值" ? <Check /> : <Copy />}
        </Button>
      </div>
      {error && (
        <p className="color-field-error" role="alert" id={errorId}>
          {error}
        </p>
      )}
      {copyState && (
        <p className="color-feedback" role="status">
          {copyState}
        </p>
      )}
      {!isSrgb(value) && (
        <p className="color-feedback">超出 sRGB，HEX 为近似值。</p>
      )}
      <div className="color-comparison">
        <button
          type="button"
          onClick={() => onCommit(original)}
          title="恢复此颜色的原版值"
          aria-label="恢复原版颜色"
          disabled={!modified}
        >
          <span style={{ background: original }} />
          <small>原版</small>
        </button>
        <div>
          <span style={{ background: value }} />
          <small>当前</small>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="重置此颜色"
          title="重置此颜色"
          disabled={!modified}
          onClick={() => onCommit(original)}
        >
          <RotateCcw />
        </Button>
      </div>
      <div className="color-presets-heading">
        快捷色 <span>sRGB</span>
      </div>
      <div className="color-presets">
        {presets.map((preset) => (
          <button
            type="button"
            key={preset}
            aria-label={`使用颜色 ${preset}`}
            title={preset}
            aria-pressed={hex === preset}
            style={{ background: preset }}
            onClick={() => onCommit(hexToOklch(preset)!)}
          />
        ))}
      </div>
      <details className="color-advanced">
        <summary>
          精确调整 <span>OKLCH</span>
          <ChevronDown className="size-3" />
        </summary>
        <div className="color-channels">
          {(
            [
              ["明度", 0, 1, 0.005],
              ["色度", 1, 0.4, 0.005],
              ["色相", 2, 360, 1],
            ] as const
          ).map(([name, index, max, step]) => (
            <label key={name}>
              <span>
                {name}
                <output>{channels[index]}</output>
              </span>
              <input
                type="range"
                aria-label={`OKLCH ${name}`}
                min={0}
                max={max}
                step={step}
                value={channels[index]}
                onChange={(event) => {
                  const next = [...channels];
                  next[index] = Number(event.target.value);
                  onCommit(`oklch(${next.join(" ")})`);
                }}
              />
            </label>
          ))}
        </div>
        <code className="color-raw-value">{value}</code>
      </details>
    </div>
  );
}
