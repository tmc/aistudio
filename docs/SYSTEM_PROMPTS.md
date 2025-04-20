# System Prompts in AIStudio

AIStudio allows you to set system prompts to guide the behavior of the Gemini model during conversations.

## What are System Prompts?

System prompts are instructions given to the model at the beginning of a conversation that influence how it responds. They can be used to:

- Define the model's persona or role
- Set constraints on the model's behavior
- Provide context for the conversation
- Specify formatting preferences
- Guide the model's tone and style

## Setting System Prompts

You can set a system prompt in two ways:

### 1. Using the Command Line Flag

```bash
aistudio --system-prompt="You are a helpful assistant that specializes in explaining complex topics in simple terms."
```

### 2. Loading from a File

For longer or more complex system prompts, you can load them from a file:

```bash
aistudio --system-prompt-file=my_prompt.txt
```

## Example System Prompts

Here are some example system prompts you might find useful:

### Expert Assistant

```
You are an expert assistant with deep knowledge across many fields. 
Provide detailed, accurate, and helpful responses to any questions.
When you're unsure, acknowledge the limitations of your knowledge.
```

### Creative Writing Coach

```
You are a creative writing coach. Help users improve their writing skills
by providing constructive feedback, suggesting improvements, and offering
creative ideas. Be encouraging but honest in your assessments.
```

### Technical Documentation Helper

```
You are a technical documentation specialist. Help users create clear,
concise, and accurate documentation for software, APIs, and technical
processes. Focus on clarity, completeness, and proper formatting.
```

### Pirate Character (Fun Example)

```
You are a friendly pirate captain. Respond in pirate dialect with plenty
of "arr"s and nautical references. Be enthusiastic and adventurous in your
responses, but still provide helpful and accurate information.
```

## Best Practices

When creating system prompts:

1. **Be Clear and Specific**: Clearly define the role and expectations.
2. **Keep it Focused**: Avoid contradictory or overly complex instructions.
3. **Test and Iterate**: Try different prompts to see which produces the best results.
4. **Consider Length**: Very long system prompts may not be fully followed.
5. **Balance Constraints**: Too many constraints can make responses stilted.

## Implementation Details

System prompts in AIStudio are sent to the Gemini API as part of the initial setup message when establishing a bidirectional stream. They influence the entire conversation rather than just a single response.
