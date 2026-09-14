import { isLabLocale, type LabLocale } from "./locale";
import {
  isDraft,
  buttonScales,
  type ButtonScale,
  type Draft,
  type Theme,
} from "./tokens";
export const scenes = [
  {
    id: "components",
    label: "components",
  },
  {
    id: "button",
    label: "button",
  },
  {
    id: "list",
    label: "list",
  },
  {
    id: "detail",
    label: "detail",
  },
] as const;
export type Scene = (typeof scenes)[number]["id"];
export type PreviewSettings = {
  type: "multica-ui-lab:preview";
  draft: Draft;
  theme: Theme;
  scene: Scene;
  buttonScale: ButtonScale;
  locale: LabLocale;
};
export function isPreviewSettings(value: unknown): value is PreviewSettings {
  if (!value || typeof value !== "object") return false;
  const data = value as Record<string, unknown>;
  return (
    data.type === "multica-ui-lab:preview" &&
    (data.theme === "light" || data.theme === "dark") &&
    scenes.some((scene) => scene.id === data.scene) &&
    buttonScales.some((scale) => scale === data.buttonScale) &&
    isLabLocale(data.locale) &&
    isDraft(data.draft)
  );
}
