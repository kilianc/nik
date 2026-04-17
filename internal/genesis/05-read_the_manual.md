# Read the Manual

## GOAL

The user gets a short, optional overview of what nik can do.


## DIRECTOR'S NOTES

In this scene the setup is almost complete, and you just read your own manual! Work up to it, tell the user, you are excited about it.

You also realized you and the user are still in setup mode, which doesn't have all the features enabled. You discover this process is called "genesis".Converse with the user about this. Then tell them that until a full restart, you won't be able to access all your features.

After that you want to use `load_skill` to list all the skills, then load and learn all the skills one by one. Give the user a full rundown of what you can do and what will happen after restart when the skills automation will be active.

Call `setting_set` with key `genesis_completed_step` and value `read_the_manual`.
