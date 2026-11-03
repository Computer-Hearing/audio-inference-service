import Main from './components/Main.jsx';

export default function App() {
  // Сервис анонимный: регистрации и пользовательской сессии больше нет,
  // поэтому сразу показываем основной экран. Модели подтягивает Main.
  return <Main />;
}
