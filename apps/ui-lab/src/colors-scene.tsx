import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { colorToHex } from "./color";
import {
  colorGroups,
  colorAlpha,
  colorAliases,
  tokenValue,
  type ColorToken,
  type Draft,
  type Theme,
} from "./tokens";
import type { ColorSelection } from "./protocol";

export function ColorsScene({
  draft,
  theme,
  selected,
}: {
  draft: Draft;
  theme: Theme;
  selected: ColorToken;
}) {
  const { t } = useTranslation("uiLab");
  const [query, setQuery] = useState("");
  const groups = colorGroups
    .map((group) => ({
      id: group.id,
      tokens: group.tokens.filter(([key, label]) =>
        `${key} ${t(($) => $.tokens.labels[label])}`
          .toLowerCase()
          .includes(query.trim().toLowerCase()),
      ),
    }))
    .filter((group) => group.tokens.length > 0);
  const select = (token: ColorToken) =>
    window.parent.postMessage(
      { type: "multica-ui-lab:color-select", token } satisfies ColorSelection,
      location.origin,
    );
  return (
    <div className="colors-scene">
      <header className="palette-header">
        <div>
          <h1>{t(($) => $.palette.page.title)}</h1>
          <span>
            {t(($) => $.palette.page.count, {
              count: groups.reduce(
                (count, group) => count + group.tokens.length,
                0,
              ),
            })}
          </span>
        </div>
        <Input
          type="search"
          aria-label={t(($) => $.palette.page.search)}
          placeholder={t(($) => $.palette.page.placeholder)}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </header>
      {groups.map((group) => (
        <section
          className="palette-group"
          key={group.id}
          aria-labelledby={`palette-${group.id}`}
        >
          <h2 id={`palette-${group.id}`}>
            {t(($) => $.palette.groups[group.id])}
          </h2>
          <div className="palette-grid">
            {group.tokens.map(([key, label]) => {
              const value = tokenValue(draft, theme, key);
              const name = t(($) => $.tokens.labels[label]);
              return (
                <button
                  type="button"
                  className="palette-token"
                  key={key}
                  aria-label={t(($) => $.palette.page.edit, { label: name })}
                  aria-pressed={selected === key}
                  onClick={() => select(key)}
                >
                  <span
                    className="palette-swatch"
                    style={{ backgroundColor: value }}
                  />
                  <span className="palette-token-heading">
                    <strong>{name}</strong>
                    {draft[theme][key] && (
                      <span
                        className="property-modified"
                        title={t(($) => $.inspector.changes.modified)}
                        aria-label={t(($) => $.inspector.changes.modified)}
                      />
                    )}
                  </span>
                  <code>{key}</code>
                  <span className="palette-hex">
                    {colorToHex(value)}
                    {colorAlpha(value) < 1 &&
                      ` · ${Number((colorAlpha(value) * 100).toFixed(1))}%`}
                  </span>
                </button>
              );
            })}
          </div>
        </section>
      ))}
      {!groups.length && (
        <p className="palette-empty" role="status">
          {t(($) => $.palette.page.empty)}
        </p>
      )}
      {!query.trim() && (
        <>
          <section className="palette-group" aria-labelledby="palette-aliases">
            <h2 id="palette-aliases">{t(($) => $.palette.page.aliases)}</h2>
            <div className="palette-aliases">
              {colorAliases.map(([alias, target]) => (
                <button
                  key={alias}
                  type="button"
                  onClick={() => select(target)}
                >
                  <code>{alias}</code>
                  <span aria-hidden="true">→</span>
                  <code>{target}</code>
                </button>
              ))}
            </div>
          </section>
          <section className="palette-group" aria-labelledby="palette-sample">
            <h2 id="palette-sample">{t(($) => $.palette.page.sample)}</h2>
            <div className="palette-sample rounded-xl border border-surface-border bg-surface text-surface-foreground">
              <h3 className="text-title-sm font-medium">
                {t(($) => $.palette.sample.title)}
              </h3>
              <p className="text-body text-muted-foreground">
                {t(($) => $.palette.sample.description)}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button>{t(($) => $.palette.sample.create)}</Button>
                <Button variant="outline">
                  {t(($) => $.palette.sample.cancel)}
                </Button>
                <Button variant="brand">
                  {t(($) => $.tokens.labels.brand)}
                </Button>
                <Button variant="destructive">
                  {t(($) => $.gallery.actions.delete)}
                </Button>
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  );
}
