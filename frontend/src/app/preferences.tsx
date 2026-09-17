import { createContext, useContext } from "react";
export type Language = "zh" | "en";
export const Preferences = createContext<{ language: Language }>({
  language: "zh",
});
export function useLanguage() {
  return useContext(Preferences).language;
}
