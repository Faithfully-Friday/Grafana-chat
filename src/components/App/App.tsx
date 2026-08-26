import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
const ChatPage = React.lazy(() => import('../../pages/ChatPage'));

function App(props: AppRootProps) {
  return (
    <Routes>
      {/* Default page */}
      <Route path="*" element={<ChatPage />} />
    </Routes>
  );
}

export default App;
