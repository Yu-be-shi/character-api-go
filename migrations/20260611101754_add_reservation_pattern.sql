-- Drop index "idx_core_characters_active_created_at" from table: "core_characters"
DROP INDEX "idx_core_characters_active_created_at";
-- Modify "core_characters" table
ALTER TABLE "core_characters" ADD COLUMN "creation_token" text NULL, ADD COLUMN "confirmed_at" timestamptz NULL;
-- Create index "idx_core_characters_active_created_at" to table: "core_characters"
CREATE INDEX "idx_core_characters_active_created_at" ON "core_characters" ("created_at", "id") WHERE ((deleted_at IS NULL) AND (confirmed_at IS NOT NULL));
-- Create index "idx_core_characters_creation_token" to table: "core_characters"
CREATE UNIQUE INDEX "idx_core_characters_creation_token" ON "core_characters" ("creation_token") WHERE (creation_token IS NOT NULL);
-- Create index "idx_core_characters_unconfirmed" to table: "core_characters"
CREATE INDEX "idx_core_characters_unconfirmed" ON "core_characters" ("created_at") WHERE (confirmed_at IS NULL);
