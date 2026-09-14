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
    label: "组件总览",
  },
  {
    id: "button",
    label: "Button 按钮",
  },
  {
    id: "list",
    label: "任务列表",
  },
  {
    id: "detail",
    label: "任务详情",
  },
] as const;
export type Scene = (typeof scenes)[number]["id"];
export type PreviewSettings = {
  type: "multica-ui-lab:preview";
  draft: Draft;
  theme: Theme;
  scene: Scene;
  buttonScale: ButtonScale;
};
export function isPreviewSettings(value: unknown): value is PreviewSettings {
  if (!value || typeof value !== "object") return false;
  const data = value as Record<string, unknown>;
  return (
    data.type === "multica-ui-lab:preview" &&
    (data.theme === "light" || data.theme === "dark") &&
    scenes.some((scene) => scene.id === data.scene) &&
    buttonScales.some((scale) => scale === data.buttonScale) &&
    isDraft(data.draft)
  );
}
