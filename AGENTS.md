# AGENTS.md

this repo uses devbox

all tooling (beyond an editor and git) should be loaded up into devbox

you can use this tooling by running `devbox run -- <the command you want to run`.

you can suggest adding tools to devbox with `devbox add <tool>`.

you can see what tools are available with `devbox search <tool>`.

this repo uses `just` as a command runner.  see the [justfile](justfile) for
existing recipes.
any new common workflow commands should be defined in a recipe in here.
