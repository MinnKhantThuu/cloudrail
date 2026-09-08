const http = require("node:http");

const port = Number.parseInt(process.env.PORT || "3000", 10);
const release = "pilot-v1";

const server = http.createServer((request, response) => {
  if (request.url === "/health") {
    response.writeHead(200, { "content-type": "application/json" });
    response.end(JSON.stringify({ status: "ok", release }));
    return;
  }

  response.writeHead(200, { "content-type": "application/json" });
  response.end(JSON.stringify({ message: "Hello from Cloudrail", release }));
});

server.listen(port, "0.0.0.0", () => {
  console.log(`cloudrail pilot listening on ${port}`);
});

const shutdown = () => server.close(() => process.exit(0));
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
