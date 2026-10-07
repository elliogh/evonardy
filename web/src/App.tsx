import { useEffect, useState } from "react";
import { Game } from "./Game";
import { Library } from "./Library";
import { Play, Settings } from "./Play";
import { Research } from "./Research";
import { Jobs } from "./Training";

const route = () =>
  typeof window === "undefined" ? "" : window.location.hash.slice(1);

export function App() {
  const [path, setPath] = useState(route);
  const [playPath, setPlayPath] = useState("");
  const gameID = path.match(/^\/games\/([A-Za-z0-9_-]+)$/)?.[1];
  const research = path.match(/^\/research(?:\/([A-Za-z0-9_-]+))?$/);
  const training = path.match(/^\/training(?:\/([A-Za-z0-9_-]+))?$/);
  const evaluation = path.match(
    /^\/evaluations(?:\/([A-Za-z0-9_-]+))?(?:\?bot=([^&]+))?$/,
  );
  const library = path === "/bots";
  const settings = path === "/settings";
  const lab = !!(research || training || evaluation || library);
  useEffect(() => {
    const changed = () => setPath(route());
    window.addEventListener("hashchange", changed);
    return () => window.removeEventListener("hashchange", changed);
  }, []);
  useEffect(() => {
    if (!lab && !settings) setPlayPath(path);
  }, [path, lab, settings]);
  return (
    <main>
      <header className="site-header">
        <a href="#" className="brand">
          <span className="mark" aria-hidden="true">
            ● ●
          </span>
          EvoNardy
        </a>
        <nav className="site-nav" aria-label="Main navigation">
          <a
            href={`#${playPath}`}
            aria-current={!lab && !settings ? "page" : undefined}
          >
            Play
          </a>
          <a href="#/training" aria-current={lab ? "page" : undefined}>
            Training
          </a>
          <a href="#/settings" aria-current={settings ? "page" : undefined}>
            Settings
          </a>
        </nav>
        <span className="local-badge">
          <i />
          Local play
        </span>
      </header>
      {lab && (
        <nav className="lab-nav" aria-label="Training navigation">
          <a href="#/bots" aria-current={library ? "page" : undefined}>
            My bots
          </a>
          <a href="#/training" aria-current={training ? "page" : undefined}>
            Runs
          </a>
          <a
            href="#/evaluations"
            aria-current={evaluation ? "page" : undefined}
          >
            Evaluate
          </a>
          <a href="#/research" aria-current={research ? "page" : undefined}>
            Research
          </a>
        </nav>
      )}
      {gameID ? (
        <Game key={gameID} id={gameID} />
      ) : research ? (
        <Research key={path} id={research[1]} />
      ) : training ? (
        <Jobs key={path} kind="training" id={training[1]} />
      ) : evaluation ? (
        <Jobs
          key={path}
          kind="evaluation"
          id={evaluation[1]}
          initialBot={evaluation[2]}
        />
      ) : library ? (
        <Library />
      ) : settings ? (
        <Settings />
      ) : (
        <Play key={path} newGame={path === "/play/new"} />
      )}
      <footer>
        <span>Long nardy · No doubling cube</span>
        <span>Ruleset long-nardy-fnr2026-nocube-v1</span>
      </footer>
    </main>
  );
}
