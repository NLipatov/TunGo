# Module dedication
The Docs module serves as a knowledge base for working with the TunGo project, whether you're using it as a user or contributing as a developer.

# Run Docs Project

## 1. Install Dependencies
```bash
npm install
```

## 2. Start the Server

**Development (single locale):**
```bash
npm start
npm start -- --locale de  # specific locale
```

**Production preview (all locales):**
```bash
npm run build && npm run serve
```

## 3. Open the Web UI

The project will be available at [http://localhost:3000](http://localhost:3000).

# Update Dependencies

```bash
npx npm-check-updates -u && npm install
```