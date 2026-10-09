import { createRoot } from 'react-dom/client';
import './style.css';

function App() {
  return (
    <main className="placeholder">
      <span className="badge">Скоро</span>
      <h1>Флагман</h1>
      <p>Склад, меню и заказы вашего ресторана в одном месте.</p>
      <p className="muted">Готовим приложение к запуску.</p>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
