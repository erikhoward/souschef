import { Icon } from './Icon.jsx';

const ingredientGroups = [
  ['For the scallion pancakes', ['1 cup (125 g) all-purpose flour', '¾ cup (180 ml) hot water', '2 scallions, thinly sliced', '½ tsp fine sea salt', '2 tbsp neutral oil, plus more for cooking']],
  ['For the chili eggs', ['4 large eggs', '2 tbsp neutral oil', '2 tbsp chili crisp', '1 tbsp soy sauce', '1 tsp rice vinegar', '½ tsp sugar', '1 scallion, sliced (green part only)', 'Toasted sesame seeds, for garnish (optional)']]
];

const instructions = [
  'Make the scallion pancakes: In a bowl, mix flour, hot water, and salt until a shaggy dough forms. Knead for 2–3 minutes until smooth, then rest for 20 minutes.',
  'Divide dough into 2 equal pieces. Roll each into a thin rectangle. Brush with oil and sprinkle with scallions. Roll up tightly and coil into a flat spiral.',
  'Roll each spiral into a 1/4-inch-thick round. Heat a thin layer of oil in a skillet over medium heat. Cook pancakes for 3–4 minutes per side until golden and crisp. Transfer to a plate and keep warm.',
  'Make the chili eggs: Heat oil in a skillet over medium-high heat. Crack in the eggs and fry until the whites are set and the edges are crispy, 2–3 minutes.',
  'Reduce heat to medium-low. Add chili crisp, soy sauce, rice vinegar, and sugar. Spoon the sauce over the eggs and cook for 1 minute, basting to coat.',
  'To serve: Slice pancakes into wedges. Top with chili eggs, scallions, and sesame seeds, if using.'
];

function RecipeFacts() {
  const facts = [['Yield', '2 servings'], ['Prep time', '20 min'], ['Cook time', '20 min'], ['Total time', '40 min']];
  return <dl className="recipe-facts">{facts.map(([label, value], index) => <div key={label}><dt>{index ? <Icon name="clock" size={22} /> : null}{label}</dt><dd>{value}</dd></div>)}</dl>;
}

function ReviewFindings() {
  return (
    <section className="review-section">
      <h3>Recipe review</h3>
      <article className="review-finding is-caution"><Icon name="lightbulb" size={27} /><div><strong>Clarify egg temperature before adding to oil</strong><p>Specify medium-high heat so the edges crisp without burning the chili crisp.</p></div><span>Medium</span></article>
      <article className="review-finding"><Icon name="check" size={25} /><strong>Ingredient balance looks good</strong><span>Passed</span></article>
      <article className="review-finding"><Icon name="check" size={25} /><strong>Steps are clear and testable</strong><span>Passed</span></article>
    </section>
  );
}

function RecipeDocument() {
  return (
    <article className="recipe-document">
      <p className="recipe-description">Jammy, crispy-edged eggs tossed in a savory chili crisp sauce and served with flaky, scallion-studded pancakes. Spicy, savory, and deeply satisfying.</p>
      <RecipeFacts />
      <div className="recipe-columns">
        <section><h2>Ingredients</h2>{ingredientGroups.map(([group, ingredients]) => <div className="ingredient-group" key={group}><h3>{group}</h3><ul>{ingredients.map((ingredient) => <li key={ingredient}>{ingredient}</li>)}</ul></div>)}</section>
        <section className="equipment-list"><h2>Equipment</h2><ul>{['Medium mixing bowl', 'Measuring cups and spoons', 'Rolling pin', '12-inch nonstick skillet or cast iron pan with lid', 'Small bowl', 'Slotted spatula'].map((item) => <li key={item}>{item}</li>)}</ul></section>
      </div>
      <section className="instruction-list"><h2>Instructions</h2><ol>{instructions.map((instruction) => <li key={instruction}>{instruction}</li>)}</ol></section>
    </article>
  );
}

function RecipeSummary() {
  return (
    <section className="recipe-summary review-section">
      <h3>Recipe summary</h3>
      <div className="summary-item"><Icon name="sparkles" size={27} /><div><h4>Elevated treatment</h4><p>Crispy-edged eggs finished in chili crisp and served on flaky scallion pancakes for contrast in texture and bold, balanced flavor.</p></div></div>
      <div className="summary-item"><Icon name="lightbulb" size={27} /><div><h4>Primary payoff</h4><p>Crunchy, spicy, savory, and satisfying—comfort food with a punch.</p></div></div>
    </section>
  );
}

export function RecipeWorkspace({ testStatus, onTestStatusChange, onApprove, onGenerateScript }) {
  return (
    <main className="workspace recipe-workspace">
      <header className="recipe-header"><h1>Crispy chili eggs with scallion pancakes</h1><span className="review-state"><i className="status-dot idea" />Review</span></header>
      <div className="recipe-layout">
        <RecipeDocument />
        <aside className="recipe-review-rail">
          <RecipeSummary />
          <ReviewFindings />
          <div className="review-controls"><label>Revision<select aria-label="Recipe revision" defaultValue="2"><option value="2">Revision 2</option><option value="1">Revision 1</option></select></label><label>Test status<select aria-label="Test status" value={testStatus} onChange={(event) => onTestStatusChange(event.target.value)}><option value="untested">Untested</option><option value="partially_tested">Partially tested</option><option value="tested">Tested</option></select></label></div>
          <button className="button button-primary approval-button" type="button" onClick={onApprove}><Icon name="check" size={20} />Approve recipe</button>
          <button className="button button-outline script-button" type="button" onClick={onGenerateScript}><Icon name="clapper" size={21} />Next: Generate 60–90 sec video script</button>
        </aside>
      </div>
      <form className="change-composer" onSubmit={(event) => event.preventDefault()}><Icon name="message" size={26} /><input placeholder="Request a change in plain language…" aria-label="Request a recipe change" /><button className="icon-button" type="submit" aria-label="Send recipe change"><Icon name="arrow" size={23} /></button></form>
    </main>
  );
}
