#!/usr/bin/env python3
"""
CodeBase Normalizer - Advanced File Aggregator
A beautiful CLI tool to aggregate codebase files into a single normalized file for AI prompts.
"""

import os
import sys
import re
import argparse
from pathlib import Path
from typing import List, Set, Dict, Optional, Tuple
from dataclasses import dataclass
from datetime import datetime
import mimetypes

try:
    from rich.console import Console
    from rich.panel import Panel
    from rich.table import Table
    from rich.tree import Tree
    from rich.prompt import Prompt, Confirm
    from rich.text import Text
    from rich.progress import Progress, SpinnerColumn, TextColumn
    from rich.layout import Layout
    from rich.align import Align
    from rich import box
    RICH_AVAILABLE = True
except ImportError:
    RICH_AVAILABLE = False
    print("⚠️  Rich library not found. Install with: pip install rich")
    print("💡 For better experience, install rich for beautiful CLI interface")

@dataclass
class FileInfo:
    """Information about a file"""
    path: Path
    relative_path: Path
    size: int
    is_text: bool
    mime_type: Optional[str] = None

class CodebaseNormalizer:
    """Main class for codebase normalization"""
    
    def __init__(self):
        self.console = Console() if RICH_AVAILABLE else None
        self.selected_files: Set[Path] = set()
        self.excluded_patterns: Set[str] = {
            # Common patterns to exclude by default
            '*.pyc', '*.pyo', '*.pyd', '__pycache__',
            '.git', '.svn', '.hg', '.bzr',
            'node_modules', '.npm', '.yarn',
            '.env', '.venv', 'venv', 'env',
            '*.log', '*.tmp', '*.temp',
            '.DS_Store', 'Thumbs.db',
            '*.exe', '*.dll', '*.so', '*.dylib',
            '*.jpg', '*.jpeg', '*.png', '*.gif', '*.bmp', '*.ico',
            '*.mp4', '*.avi', '*.mov', '*.wmv', '*.flv',
            '*.mp3', '*.wav', '*.flac', '*.aac',
            '*.zip', '*.rar', '*.7z', '*.tar', '*.gz',
            '*.pdf', '*.doc', '*.docx', '*.xls', '*.xlsx'
        }
        self.common_code_extensions = {
            '.py', '.js', '.ts', '.jsx', '.tsx', '.java', '.cpp', '.c', '.h',
            '.cs', '.php', '.rb', '.go', '.rs', '.swift', '.kt', '.scala',
            '.html', '.css', '.scss', '.sass', '.less', '.xml', '.json',
            '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf', '.sh', '.bat',
            '.sql', '.md', '.txt', '.rst', '.tex', '.r', '.m', '.pl', '.lua'
        }
    
    def print_banner(self):
        """Print application banner"""
        if not RICH_AVAILABLE:
            print("=" * 60)
            print("🚀 CodeBase Normalizer - Advanced File Aggregator")
            print("=" * 60)
            return
        
        banner = Panel.fit(
            "[bold blue]🚀 CodeBase Normalizer[/bold blue]\n"
            "[dim]Advanced File Aggregator for AI Prompts[/dim]\n"
            "[green]✨ Beautiful CLI • 🎯 Smart Selection • 🌳 Tree View • 📝 Normalized Output[/green]",
            box=box.DOUBLE,
            border_style="blue"
        )
        self.console.print(banner)
        self.console.print()
    
    def get_directory_path(self) -> Path:
        """Get directory path from user input"""
        while True:
            if RICH_AVAILABLE:
                path_input = Prompt.ask(
                    "[bold cyan]Enter the directory path[/bold cyan]",
                    default="."
                )
            else:
                path_input = input("Enter the directory path (default: current directory): ") or "."
            
            path = Path(path_input).expanduser().resolve()
            
            if not path.exists():
                self.print_error(f"Path does not exist: {path}")
                continue
            
            if not path.is_dir():
                self.print_error(f"Path is not a directory: {path}")
                continue
            
            return path
    
    def print_error(self, message: str):
        """Print error message"""
        if RICH_AVAILABLE:
            self.console.print(f"[bold red]❌ Error:[/bold red] {message}")
        else:
            print(f"❌ Error: {message}")
    
    def print_success(self, message: str):
        """Print success message"""
        if RICH_AVAILABLE:
            self.console.print(f"[bold green]✅ Success:[/bold green] {message}")
        else:
            print(f"✅ Success: {message}")
    
    def print_info(self, message: str):
        """Print info message"""
        if RICH_AVAILABLE:
            self.console.print(f"[bold blue]ℹ️  Info:[/bold blue] {message}")
        else:
            print(f"ℹ️  Info: {message}")
    
    def is_text_file(self, file_path: Path) -> bool:
        """Check if file is likely a text file"""
        try:
            # Check by extension first
            if file_path.suffix.lower() in self.common_code_extensions:
                return True
            
            # Check MIME type
            mime_type, _ = mimetypes.guess_type(str(file_path))
            if mime_type and mime_type.startswith('text/'):
                return True
            
            # Try to read first few bytes
            with open(file_path, 'rb') as f:
                chunk = f.read(1024)
                if not chunk:
                    return True
                # Check for null bytes (binary files usually contain them)
                return b'\x00' not in chunk
        
        except (OSError, IOError):
            return False
    
    def should_exclude_path(self, path: Path, base_path: Path) -> bool:
        """Check if path should be excluded based on patterns"""
        relative_path = path.relative_to(base_path)
        path_str = str(relative_path)
        
        for pattern in self.excluded_patterns:
            if self.match_pattern(path_str, pattern) or self.match_pattern(path.name, pattern):
                return True
        return False
    
    def match_pattern(self, text: str, pattern: str) -> bool:
        """Match text against pattern (supports wildcards)"""
        # Convert shell-style wildcards to regex
        regex_pattern = pattern.replace('*', '.*').replace('?', '.')
        return bool(re.match(f'^{regex_pattern}$', text, re.IGNORECASE))
    
    def scan_directory(self, directory: Path) -> Dict[str, List[FileInfo]]:
        """Scan directory and return organized file information"""
        files_info = {'files': [], 'directories': []}
        
        try:
            for item in sorted(directory.iterdir()):
                if self.should_exclude_path(item, directory):
                    continue
                
                if item.is_file():
                    size = item.stat().st_size
                    is_text = self.is_text_file(item)
                    mime_type, _ = mimetypes.guess_type(str(item))
                    
                    file_info = FileInfo(
                        path=item,
                        relative_path=item.relative_to(directory),
                        size=size,
                        is_text=is_text,
                        mime_type=mime_type
                    )
                    files_info['files'].append(file_info)
                
                elif item.is_dir():
                    file_info = FileInfo(
                        path=item,
                        relative_path=item.relative_to(directory),
                        size=0,
                        is_text=False
                    )
                    files_info['directories'].append(file_info)
        
        except PermissionError:
            self.print_error(f"Permission denied accessing: {directory}")
        
        return files_info
    
    def format_size(self, size: int) -> str:
        """Format file size in human-readable format"""
        for unit in ['B', 'KB', 'MB', 'GB']:
            if size < 1024:
                return f"{size:.1f} {unit}"
            size /= 1024
        return f"{size:.1f} TB"
    
    def clear_screen(self):
        """Clear the screen"""
        os.system('cls' if os.name == 'nt' else 'clear')
    
    def display_status_header(self, current_path: Path):
        """Display status header with current path and selection count"""
        if not RICH_AVAILABLE:
            print("=" * 80)
            print(f"📍 Current Path: {current_path}")
            print(f"📋 Selected Files: {len(self.selected_files)}")
            print("=" * 80)
            return
        
        # Rich version
        status_panel = Panel(
            f"[bold cyan]📍 Current Path:[/bold cyan] [white]{current_path}[/white]\n"
            f"[bold green]📋 Selected Files:[/bold green] [yellow]{len(self.selected_files)}[/yellow]",
            box=box.ROUNDED,
            border_style="blue",
            title="[bold blue]Status[/bold blue]"
        )
        self.console.print(status_panel)
    
    def display_directory_contents(self, directory: Path, files_info: Dict[str, List[FileInfo]]):
        """Display directory contents in a beautiful table with numbers"""
        all_items = files_info['directories'] + files_info['files']
        
        if not RICH_AVAILABLE:
            print(f"\n📁 Directory Contents ({len(all_items)} items):")
            print("-" * 60)
            
            for i, item in enumerate(all_items, 1):
                if item.path.is_dir():
                    print(f"  {i:2d}. 📂 {item.path.name}/")
                else:
                    icon = "📝" if item.is_text else "📄"
                    size_str = self.format_size(item.size)
                    selected = "✅" if item.path in self.selected_files else "  "
                    print(f"  {i:2d}. {selected} {icon} {item.path.name} ({size_str})")
            return
        
        # Rich version
        if not all_items:
            self.console.print("[dim]📁 Directory is empty[/dim]")
            return
        
        table = Table(
            box=box.ROUNDED, 
            show_header=True, 
            header_style="bold magenta",
            title=f"📁 Directory Contents ({len(all_items)} items)"
        )
        table.add_column("#", style="dim", width=4, justify="right")
        table.add_column("Sel", style="green", width=4, justify="center")
        table.add_column("Type", style="cyan", width=6)
        table.add_column("Name", style="white")
        table.add_column("Size", justify="right", style="yellow", width=10)
        table.add_column("Info", style="dim")
        
        # Add all items with numbers
        for i, item in enumerate(all_items, 1):
            if item.path.is_dir():
                table.add_row(
                    str(i),
                    "",
                    "📂 DIR",
                    f"[bold]{item.path.name}/[/bold]",
                    "-",
                    "Directory"
                )
            else:
                icon = "📝 TXT" if item.is_text else "📄 BIN"
                color = "green" if item.is_text else "red"
                selected_mark = "✅" if item.path in self.selected_files else ""
                
                table.add_row(
                    str(i),
                    selected_mark,
                    f"[{color}]{icon}[/{color}]",
                    item.path.name,
                    self.format_size(item.size),
                    item.mime_type or "Unknown"
                )
        
        self.console.print(table)
    
    def show_selection_menu(self) -> str:
        """Show selection menu and get user choice"""
        if not RICH_AVAILABLE:
            print("\n" + "=" * 50)
            print("📋 Selection Options:")
            print("1. Select individual files/directories")
            print("2. Select all text files")
            print("3. Select all files")
            print("4. Open directory")
            print("5. Add regex pattern")
            print("6. Remove selection")
            print("7. Show current selection")
            print("8. Manage exclusions")
            print("9. Generate normalized file")
            print("0. Back/Exit")
            return input("Choose an option: ")
        
        # Rich version
        menu_panel = Panel(
            "[bold cyan]📋 Selection Options[/bold cyan]\n\n"
            "[green]1.[/green] Select individual files/directories\n"
            "[green]2.[/green] Select all text files\n"
            "[green]3.[/green] Select all files\n"
            "[green]4.[/green] Open directory\n"
            "[green]5.[/green] Add regex pattern\n"
            "[green]6.[/green] Remove selection\n"
            "[green]7.[/green] Show current selection\n"
            "[green]8.[/green] Manage exclusions\n"
            "[green]9.[/green] Generate normalized file\n"
            "[green]0.[/green] Back/Exit",
            box=box.ROUNDED,
            border_style="cyan"
        )
        
        self.console.print(menu_panel)
        return Prompt.ask("[bold yellow]Choose an option[/bold yellow]", choices=list("0123456789"))
    
    def select_individual_items(self, files_info: Dict[str, List[FileInfo]]):
        """Allow user to select individual files/directories"""
        all_items = files_info['directories'] + files_info['files']
        
        if not all_items:
            self.print_info("No items to select")
            return
        
        while True:
            if RICH_AVAILABLE:
                self.console.print("\n[bold yellow]💡 Selection Help:[/bold yellow]")
                self.console.print("[dim]• Enter numbers: 1,3,5 or 1-5 for range[/dim]")
                self.console.print("[dim]• Type 'all' to select all items[/dim]")
                self.console.print("[dim]• Type 'text' to select only text files[/dim]")
                self.console.print("[dim]• Type 'done' to finish selection[/dim]")
            else:
                print("\n💡 Selection Help:")
                print("• Enter numbers: 1,3,5 or 1-5 for range")
                print("• Type 'all' to select all items")
                print("• Type 'text' to select only text files")
                print("• Type 'done' to finish selection")
            
            user_input = input("\n🎯 Selection: ").strip().lower()
            
            if user_input in ['done', 'exit', 'quit', '']:
                break
            
            if user_input == 'all':
                for item in all_items:
                    if item.is_text or item.path.is_dir():
                        self.selected_files.add(item.path)
                self.print_success(f"Selected all {len(all_items)} items")
                continue
            
            if user_input == 'text':
                count = 0
                for item in all_items:
                    if item.is_text:
                        self.selected_files.add(item.path)
                        count += 1
                self.print_success(f"Selected {count} text files")
                continue
            
            try:
                # Parse input (support ranges like 1-5 and individual numbers like 1,3,5)
                indices = []
                parts = user_input.split(',')
                
                for part in parts:
                    part = part.strip()
                    if '-' in part and part.count('-') == 1:
                        # Handle range like 1-5
                        start, end = part.split('-')
                        start, end = int(start.strip()), int(end.strip())
                        indices.extend(range(start, end + 1))
                    else:
                        # Handle individual number
                        indices.append(int(part))
                
                # Remove duplicates and convert to 0-based indices
                indices = list(set(idx - 1 for idx in indices))
                selected_count = 0
                
                for idx in indices:
                    if 0 <= idx < len(all_items):
                        item = all_items[idx]
                        if item.path.is_file() and item.is_text:
                            self.selected_files.add(item.path)
                            selected_count += 1
                            if RICH_AVAILABLE:
                                self.console.print(f"[green]✅ Selected:[/green] {item.path.name}")
                            else:
                                print(f"✅ Selected: {item.path.name}")
                        elif item.path.is_dir():
                            # Add all text files from directory recursively
                            added = self.add_directory_files(item.path)
                            selected_count += added
                            if RICH_AVAILABLE:
                                self.console.print(f"[green]✅ Selected directory:[/green] {item.path.name}/ ({added} files)")
                            else:
                                print(f"✅ Selected directory: {item.path.name}/ ({added} files)")
                        else:
                            if RICH_AVAILABLE:
                                self.console.print(f"[yellow]⚠️  Skipped (not text):[/yellow] {item.path.name}")
                            else:
                                print(f"⚠️  Skipped (not text): {item.path.name}")
                    else:
                        if RICH_AVAILABLE:
                            self.console.print(f"[red]❌ Invalid number:[/red] {idx + 1}")
                        else:
                            print(f"❌ Invalid number: {idx + 1}")
                
                if selected_count > 0:
                    self.print_success(f"Total selected: {selected_count} items")
                else:
                    self.print_info("No valid text files selected")
            
            except ValueError:
                self.print_error("Invalid input. Please enter numbers separated by commas or ranges like 1-5")
            except Exception as e:
                self.print_error(f"Error processing selection: {e}")
    
    def add_directory_files(self, directory: Path) -> int:
        """Recursively add all text files from directory"""
        count = 0
        try:
            for item in directory.rglob('*'):
                if item.is_file() and not self.should_exclude_path(item, directory.parent):
                    if self.is_text_file(item):
                        self.selected_files.add(item)
                        count += 1
        except PermissionError:
            self.print_error(f"Permission denied accessing: {directory}")
        
        return count
    
    def select_by_regex(self, files_info: Dict[str, List[FileInfo]], base_path: Path):
        """Select files using regex pattern"""
        if RICH_AVAILABLE:
            pattern = Prompt.ask("[bold yellow]Enter regex pattern to match filenames[/bold yellow]")
        else:
            pattern = input("Enter regex pattern to match filenames: ")
        
        try:
            regex = re.compile(pattern, re.IGNORECASE)
            selected_count = 0
            
            # Check current directory files
            for file_info in files_info['files']:
                if regex.search(file_info.path.name) and file_info.is_text:
                    self.selected_files.add(file_info.path)
                    selected_count += 1
            
            # Check subdirectories recursively
            for item in base_path.rglob('*'):
                if item.is_file() and not self.should_exclude_path(item, base_path):
                    if regex.search(item.name) and self.is_text_file(item):
                        self.selected_files.add(item)
                        selected_count += 1
            
            self.print_success(f"Selected {selected_count} files matching pattern '{pattern}'")
        
        except re.error as e:
            self.print_error(f"Invalid regex pattern: {e}")
    
    def show_current_selection(self):
        """Display currently selected files"""
        if not self.selected_files:
            self.print_info("No files selected")
            return
        
        if not RICH_AVAILABLE:
            print(f"\n📋 Selected Files ({len(self.selected_files)}):")
            print("-" * 50)
            for i, file_path in enumerate(sorted(self.selected_files), 1):
                size = self.format_size(file_path.stat().st_size)
                print(f"  {i}. {file_path} ({size})")
            return
        
        # Rich version
        table = Table(title=f"📋 Selected Files ({len(self.selected_files)})", box=box.ROUNDED)
        table.add_column("#", style="dim", width=4)
        table.add_column("File Path", style="cyan")
        table.add_column("Size", justify="right", style="yellow")
        
        for i, file_path in enumerate(sorted(self.selected_files), 1):
            size = self.format_size(file_path.stat().st_size)
            table.add_row(str(i), str(file_path), size)
        
        self.console.print(table)
    
    def manage_exclusions(self):
        """Manage exclusion patterns"""
        while True:
            if not RICH_AVAILABLE:
                print(f"\n🚫 Current Exclusion Patterns ({len(self.excluded_patterns)}):")
                for i, pattern in enumerate(sorted(self.excluded_patterns), 1):
                    print(f"  {i}. {pattern}")
                print("\nOptions: (a)dd, (r)emove, (c)lear, (d)one")
                choice = input("Choice: ").lower()
            else:
                # Rich version
                table = Table(title=f"🚫 Exclusion Patterns ({len(self.excluded_patterns)})", box=box.ROUNDED)
                table.add_column("#", width=4)
                table.add_column("Pattern", style="red")
                
                for i, pattern in enumerate(sorted(self.excluded_patterns), 1):
                    table.add_row(str(i), pattern)
                
                self.console.print(table)
                choice = Prompt.ask(
                    "[bold yellow]Options[/bold yellow]",
                    choices=['a', 'r', 'c', 'd'],
                    show_choices=True
                ).lower()
            
            if choice in ['d', 'done']:
                break
            elif choice in ['a', 'add']:
                pattern = input("Enter exclusion pattern: ").strip()
                if pattern:
                    self.excluded_patterns.add(pattern)
                    self.print_success(f"Added exclusion pattern: {pattern}")
            elif choice in ['r', 'remove']:
                try:
                    idx = int(input("Enter pattern number to remove: ")) - 1
                    patterns_list = sorted(self.excluded_patterns)
                    if 0 <= idx < len(patterns_list):
                        removed = patterns_list[idx]
                        self.excluded_patterns.remove(removed)
                        self.print_success(f"Removed exclusion pattern: {removed}")
                except (ValueError, IndexError):
                    self.print_error("Invalid pattern number")
            elif choice in ['c', 'clear']:
                if RICH_AVAILABLE:
                    if Confirm.ask("Clear all exclusion patterns?"):
                        self.excluded_patterns.clear()
                        self.print_success("Cleared all exclusion patterns")
                else:
                    confirm = input("Clear all exclusion patterns? (y/N): ").lower()
                    if confirm in ['y', 'yes']:
                        self.excluded_patterns.clear()
                        self.print_success("Cleared all exclusion patterns")
    
    def generate_tree_structure(self, base_path: Path) -> str:
        """Generate tree structure for selected files"""
        if not self.selected_files:
            return ""
        
        # Create a tree structure
        tree_lines = []
        tree_lines.append(f"📁 {base_path.name}/")
        
        # Group files by directory
        dir_files = {}
        for file_path in sorted(self.selected_files):
            try:
                rel_path = file_path.relative_to(base_path)
                dir_path = rel_path.parent
                if dir_path not in dir_files:
                    dir_files[dir_path] = []
                dir_files[dir_path].append(rel_path.name)
            except ValueError:
                # File is outside base path
                continue
        
        # Generate tree representation
        for dir_path in sorted(dir_files.keys()):
            if str(dir_path) != '.':
                tree_lines.append(f"├── 📁 {dir_path}/")
            
            files = sorted(dir_files[dir_path])
            for i, filename in enumerate(files):
                is_last = i == len(files) - 1 and (str(dir_path) == '.' or dir_path == list(dir_files.keys())[-1])
                prefix = "    └── " if str(dir_path) != '.' else "├── "
                if is_last and str(dir_path) == '.' and dir_path == list(dir_files.keys())[-1]:
                    prefix = "└── "
                tree_lines.append(f"{prefix}📝 {filename}")
        
        return "\n".join(tree_lines)
    
    def generate_normalized_file(self, base_path: Path, output_file: str = "NormalizedFile.txt"):
        """Generate the normalized file with all selected code"""
        if not self.selected_files:
            self.print_error("No files selected")
            return
        
        output_path = Path(output_file)
        
        try:
            with open(output_path, 'w', encoding='utf-8') as outfile:
                # Write header
                outfile.write("=" * 80 + "\n")
                outfile.write("🚀 CODEBASE NORMALIZED FILE\n")
                outfile.write("=" * 80 + "\n")
                outfile.write(f"Generated on: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                outfile.write(f"Base Directory: {base_path}\n")
                outfile.write(f"Total Files: {len(self.selected_files)}\n")
                outfile.write("=" * 80 + "\n\n")
                
                # Write tree structure
                outfile.write("📋 PROJECT STRUCTURE\n")
                outfile.write("-" * 40 + "\n")
                tree_structure = self.generate_tree_structure(base_path)
                if tree_structure:
                    outfile.write(tree_structure + "\n\n")
                
                outfile.write("=" * 80 + "\n")
                outfile.write("📄 FILE CONTENTS\n")
                outfile.write("=" * 80 + "\n\n")
                
                # Process each selected file
                successful_files = 0
                failed_files = 0
                
                for file_path in sorted(self.selected_files):
                    try:
                        # Get relative path for display
                        try:
                            rel_path = file_path.relative_to(base_path)
                        except ValueError:
                            rel_path = file_path
                        
                        # Write file header
                        outfile.write("/" + "=" * 78 + "\\\n")
                        outfile.write(f"| FILE: {rel_path}\n")
                        outfile.write(f"| PATH: {file_path}\n")
                        outfile.write(f"| SIZE: {self.format_size(file_path.stat().st_size)}\n")
                        outfile.write("\\" + "=" * 78 + "/\n\n")
                        
                        # Read and write file content
                        try:
                            with open(file_path, 'r', encoding='utf-8') as infile:
                                content = infile.read()
                                outfile.write(content)
                                if not content.endswith('\n'):
                                    outfile.write('\n')
                            successful_files += 1
                        except UnicodeDecodeError:
                            # Try with different encoding
                            try:
                                with open(file_path, 'r', encoding='latin-1') as infile:
                                    content = infile.read()
                                    outfile.write(f"[WARNING: File decoded with latin-1 encoding]\n")
                                    outfile.write(content)
                                    if not content.endswith('\n'):
                                        outfile.write('\n')
                                successful_files += 1
                            except Exception as e:
                                outfile.write(f"[ERROR: Could not read file - {e}]\n")
                                failed_files += 1
                        except Exception as e:
                            outfile.write(f"[ERROR: Could not read file - {e}]\n")
                            failed_files += 1
                        
                        outfile.write("\n" + "-" * 80 + "\n\n")
                    
                    except Exception as e:
                        self.print_error(f"Error processing {file_path}: {e}")
                        failed_files += 1
                
                # Write footer
                outfile.write("=" * 80 + "\n")
                outfile.write("📊 GENERATION SUMMARY\n")
                outfile.write("=" * 80 + "\n")
                outfile.write(f"✅ Successfully processed: {successful_files} files\n")
                if failed_files > 0:
                    outfile.write(f"❌ Failed to process: {failed_files} files\n")
                outfile.write(f"📁 Base directory: {base_path}\n")
                outfile.write(f"💾 Output file: {output_path.absolute()}\n")
                outfile.write(f"🕒 Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                outfile.write("=" * 80 + "\n")
            
            self.print_success(f"Normalized file generated: {output_path.absolute()}")
            self.print_info(f"Processed {successful_files} files successfully")
            if failed_files > 0:
                self.print_error(f"Failed to process {failed_files} files")
        
        except Exception as e:
            self.print_error(f"Failed to create normalized file: {e}")
    
    def navigate_directory(self, directory: Path):
        """Navigate and interact with directory contents"""
        while True:
            # Clear screen and show status
            self.clear_screen()
            self.display_status_header(directory)
            
            files_info = self.scan_directory(directory)
            self.display_directory_contents(directory, files_info)
            
            choice = self.show_selection_menu()
            
            if choice == '0':  # Back/Exit
                break
            elif choice == '1':  # Select individual
                self.select_individual_items(files_info)
                input("\n⏸️  Press Enter to continue...")
            elif choice == '2':  # Select all text files
                count = 0
                for file_info in files_info['files']:
                    if file_info.is_text:
                        self.selected_files.add(file_info.path)
                        count += 1
                self.print_success(f"Selected {count} text files")
                input("\n⏸️  Press Enter to continue...")
            elif choice == '3':  # Select all files
                count = 0
                for file_info in files_info['files']:
                    self.selected_files.add(file_info.path)
                    count += 1
                self.print_success(f"Selected {count} files")
                input("\n⏸️  Press Enter to continue...")
            elif choice == '4':  # Open directory
                if not files_info['directories']:
                    self.print_info("No directories to open")
                    input("\n⏸️  Press Enter to continue...")
                    continue
                
                # Show numbered directories for selection
                all_items = files_info['directories'] + files_info['files']
                dirs_only = files_info['directories']
                
                if RICH_AVAILABLE:
                    self.console.print("\n[bold yellow]📂 Select directory to open:[/bold yellow]")
                else:
                    print("\n📂 Select directory to open:")
                
                for i, dir_info in enumerate(dirs_only, 1):
                    # Find the actual index in all_items
                    actual_index = all_items.index(dir_info) + 1
                    print(f"  {actual_index}. 📂 {dir_info.path.name}/")
                
                try:
                    dir_input = input("\n🎯 Directory number (or Enter to cancel): ").strip()
                    if not dir_input:
                        continue
                        
                    dir_choice = int(dir_input) - 1
                    if 0 <= dir_choice < len(all_items) and all_items[dir_choice].path.is_dir():
                        selected_dir = all_items[dir_choice].path
                        self.navigate_directory(selected_dir)
                    else:
                        self.print_error("Invalid directory number or not a directory")
                        input("\n⏸️  Press Enter to continue...")
                except ValueError:
                    self.print_error("Invalid directory number")
                    input("\n⏸️  Press Enter to continue...")
            elif choice == '5':  # Regex selection
                self.select_by_regex(files_info, directory)
                input("\n⏸️  Press Enter to continue...")
            elif choice == '6':  # Remove selection
                if self.selected_files:
                    self.selected_files.clear()
                    self.print_success("Cleared all selections")
                else:
                    self.print_info("No files selected")
                input("\n⏸️  Press Enter to continue...")
            elif choice == '7':  # Show selection
                self.clear_screen()
                self.display_status_header(directory)
                self.show_current_selection()
                input("\n⏸️  Press Enter to continue...")
            elif choice == '8':  # Manage exclusions
                self.manage_exclusions()
                input("\n⏸️  Press Enter to continue...")
            elif choice == '9':  # Generate file
                if not self.selected_files:
                    self.print_error("No files selected")
                    input("\n⏸️  Press Enter to continue...")
                    continue
                
                if RICH_AVAILABLE:
                    output_name = Prompt.ask(
                        "[bold yellow]Output filename[/bold yellow]",
                        default="NormalizedFile.txt"
                    )
                else:
                    output_name = input("Output filename (default: NormalizedFile.txt): ") or "NormalizedFile.txt"
                
                self.generate_normalized_file(directory, output_name)
                
                if RICH_AVAILABLE:
                    if Confirm.ask("Exit after generation?"):
                        return
                else:
                    if input("Exit after generation? (y/N): ").lower() in ['y', 'yes']:
                        return
                
                input("\n⏸️  Press Enter to continue...")
    
    def run(self):
        """Main application loop"""
        self.print_banner()
        
        try:
            # Get directory path
            directory = self.get_directory_path()
            self.print_success(f"Selected directory: {directory}")
            
            # Start navigation
            self.navigate_directory(directory)
            
        except KeyboardInterrupt:
            if RICH_AVAILABLE:
                self.console.print("\n[bold red]👋 Goodbye![/bold red]")
            else:
                print("\n👋 Goodbye!")
        except Exception as e:
            self.print_error(f"Unexpected error: {e}")

def main():
    """Entry point for the application"""
    parser = argparse.ArgumentParser(
        description="CodeBase Normalizer - Advanced File Aggregator for AI Prompts",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python codebase_normalizer.py
  python codebase_normalizer.py --help
  
Features:
  ✨ Beautiful CLI interface with Rich library
  🎯 Smart file selection with regex support
  🌳 Tree structure generation
  📝 Normalized output for AI prompts
  🚫 Advanced exclusion patterns
  📊 File size and type detection
  🔄 Recursive directory navigation
        """
    )
    
    parser.add_argument(
        "--version",
        action="version",
        version="CodeBase Normalizer v2.0.0"
    )
    
    parser.add_argument(
        "--no-color",
        action="store_true",
        help="Disable colored output (fallback mode)"
    )
    
    args = parser.parse_args()
    
    # Override Rich availability if no-color is specified
    if args.no_color:
        global RICH_AVAILABLE
        RICH_AVAILABLE = False
    
    # Check for Rich library and provide helpful message
    if not RICH_AVAILABLE:
        print("🚀 CodeBase Normalizer - Advanced File Aggregator")
        print("=" * 60)
        print("💡 For the best experience, install the Rich library:")
        print("   pip install rich")
        print("   This will enable beautiful colored CLI interface!")
        print("=" * 60)
        print()
    
    # Create and run the normalizer
    normalizer = CodebaseNormalizer()
    normalizer.run()

if __name__ == "__main__":
    main()