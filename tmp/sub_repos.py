#!/usr/bin/env python3

import os
import sys
import subprocess

def find_git_repos(base_dir, max_depth, only_dirty=False):
    """Recursively finds git repositories up to a certain depth."""
    repos = []
    base_dir = os.path.abspath(base_dir)
    
    for dirpath, dirnames, filenames in os.walk(base_dir):
        # Prevent os.walk from searching inside .git folders
        if '.git' in dirnames:
            dirnames.remove('.git')
            
        # Calculate current depth
        rel_path = os.path.relpath(dirpath, base_dir)
        depth = 0 if rel_path == '.' else rel_path.count(os.sep) + 1
        
        if depth > max_depth:
            dirnames.clear() # Stop digging deeper in this branch
            continue

        # If it's a git repo (and not the root repo)
        if os.path.isdir(os.path.join(dirpath, '.git')):
            if dirpath != base_dir:
                # Store the relative path (e.g., 'addons/my_addon')
                repos.append(os.path.relpath(dirpath, base_dir))
                
    
    if not only_dirty:
        return repos

    dirty = []
    for path in repos:
        full_path = os.path.join(base_dir, path)
        result = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=full_path,
            capture_output=True
        )
        if result.stdout.strip():
            dirty.append(path)
        

    return dirty


def _header(text):
    print(f"\n{'-'*50}")
    print(f"📁 {text}")
    print(f"{'-'*50}")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python3 sub_repos.py <base_dir> \"<command>\"")
        print("Example: python3 sub_repos.py . \"git status | grep 'changes'\"")
        sys.exit(1)

    base_dir = sys.argv[1]
    capture_output = sys.argv[2] == "1"
    dirty = sys.argv[3] == "1"

    depth = 5
    
    # Take the rest of the arguments and join them into a single string
    # e.g., ["git", "status", "|", "grep", "changes"] -> "git status | grep changes"
    command_string = " ".join(sys.argv[4:]) 
    
    repos = find_git_repos(base_dir, depth, dirty)
    
    for path in repos:
        display_path = os.path.join(os.path.basename(base_dir), path)
        full_path = os.path.join(base_dir, path)
        
        
        try:
            if not capture_output:
                _header(display_path)

            # Notice shell=True and we pass the string, not a list
            result = subprocess.run(
                command_string, 
                cwd=full_path,
                shell=True,          # <--- Allows pipes, &&, >>, etc.
                capture_output=capture_output, 
                text=True
            )
            
            # ONLY print the header if the command actually outputted something.
            # This prevents your screen from filling up with empty headers
            # when grep filters out all the output.
            if capture_output and (result.stdout.strip() or result.stderr.strip()):
                _header(display_path)

                if result.stdout.strip():
                    print(result.stdout.strip())
                if result.stderr.strip():
                    print(result.stderr.strip(), file=sys.stderr)
                
        except Exception as e:
            print(f"Error running command in {full_path}: {e}")
        

    
    