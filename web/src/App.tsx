export function App() {
  return (
    <main>
      <header><span className="mark" aria-hidden="true">● ●</span><p>EvoNardy</p></header>
      <p className="eyebrow">Long nardy · Runs locally</p>
      <h1>Play. Train.<br />Compare.</h1>
      <p className="intro">A rules-first foundation: a correct game engine, two baseline bots, and reproducible games.</p>
      <section aria-labelledby="status"><h2 id="status">Current milestone: engine and simulations</h2>
        <p>Browser play, the model library, and training are not available yet. Run Random and Heuristic simulations through the CLI, and verify saved games with the replay command.</p>
      </section>
      <footer>Ruleset long-nardy-fnr2026-nocube-v1 · No doubling cube</footer>
    </main>
  );
}
