-- CreateTable
CREATE TABLE if not exists "delegates" (
    "addr" VARCHAR(42) NOT NULL,
    "delegate" VARCHAR(42) NOT NULL,
    "block" BIGINT NOT NULL,
    "index" VARCHAR(42) NOT NULL,
    "to_block" BIGINT NOT NULL,
    "chain_id" INT NOT NULL,
    "created_on" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "delegates_pkey" PRIMARY KEY ("addr", "block")
);

-- CreateIndex
CREATE INDEX IF NOT EXISTS "addr_idx" ON "delegates"("addr");
CREATE INDEX IF NOT EXISTS "delegate_idx" ON "delegates"("delegate");
CREATE INDEX IF NOT EXISTS "block_idx" ON "delegates"("to_block");
CREATE INDEX IF NOT EXISTS "to_block_idx" ON "delegates"("to_block");