-- Drop index "idx_core_characters_gender" from table: "core_characters"
DROP INDEX "idx_core_characters_gender";
-- Create index "idx_core_characters_active_created_at" to table: "core_characters"
CREATE INDEX "idx_core_characters_active_created_at" ON "core_characters" ("created_at", "id") WHERE (deleted_at IS NULL);
