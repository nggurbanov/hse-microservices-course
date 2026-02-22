from fastapi import FastAPI

app = FastAPI(title="Catalog Service", version="1.0.0")


@app.get("/health")
async def health_check():
    return {"status": "ok"}
