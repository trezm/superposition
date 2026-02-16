self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const sessionId = e.notification.data?.sessionId;
  const url = sessionId ? `/sessions/${sessionId}` : "/sessions";
  e.waitUntil(
    clients.matchAll({ type: "window" }).then((windowClients) => {
      for (const client of windowClients) {
        if ("focus" in client) {
          client.navigate(url);
          return client.focus();
        }
      }
      if (clients.openWindow) {
        return clients.openWindow(url);
      }
    }),
  );
});
