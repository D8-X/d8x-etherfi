-- CreateTable
CREATE TABLE if not exists "sh_tkn_transfer" (
    "from" VARCHAR(42) NOT NULL,
    "to" VARCHAR(42) NOT NULL,
    "block" BIGINT NOT NULL,
    "to_block" BIGINT NOT NULL,
    "sh_tkn" VARCHAR(42) NOT NULL,
    "chain_id" INT NOT NULL,
    "created_on" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "sh_tkn_transfer_pkey" PRIMARY KEY ("from", "to", "block", "sh_tkn", "created_on")
);

-- CreateIndex
CREATE INDEX IF NOT EXISTS "from_idx" ON "sh_tkn_transfer"("from");
CREATE INDEX IF NOT EXISTS "to_idx" ON "sh_tkn_transfer"("to");