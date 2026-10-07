import { api, command } from "./api";
import type { Player, Snapshot } from "./api";

type Preference = "bot" | "color" | "game";
const session = new Map<Preference, string>();
const unsaved = new Set<Preference>();

export function recalled(key: Preference): string | null {
  if (typeof window === "undefined") return null;
  if (unsaved.has(key)) return session.get(key) ?? null;
  try {
    return (
      window.localStorage.getItem(`evonardy.${key}`) ?? session.get(key) ?? null
    );
  } catch {
    return session.get(key) ?? null;
  }
}

export function remember(key: Preference, value: string) {
  session.set(key, value);
  try {
    window.localStorage.setItem(`evonardy.${key}`, value);
    unsaved.delete(key);
  } catch {
    // Readable storage may still reject writes (for example, when full).
    unsaved.add(key);
  }
}

export const preferredColor = (): Player => (recalled("color") === "1" ? 1 : 0);

export async function startGame(botID: string, human = preferredColor()) {
  const game = await api<Snapshot>("/games", "POST", {
    ...command(0),
    bot_id: botID,
    human,
  });
  remember("bot", botID);
  remember("game", game.id);
  window.location.hash = `/games/${game.id}`;
}
