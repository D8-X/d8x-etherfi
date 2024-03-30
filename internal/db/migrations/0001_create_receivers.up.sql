-- CreateTable
CREATE TABLE if not exists "receivers" (
    "addr" VARCHAR(42) NOT NULL,
    "from_block" BIGINT NOT NULL,
    "to_block" BIGINT NOT NULL,
    "pool_tkn" VARCHAR(42) NOT NULL,
    "created_on" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "receivers_pkey" PRIMARY KEY ("addr", "from_block", "to_block", "pool_tkn")
);

-- CreateIndex
CREATE INDEX IF NOT EXISTS "addr_idx" ON "receivers"("addr");